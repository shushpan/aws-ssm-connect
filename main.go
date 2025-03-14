package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2Types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/sso"
	ssoTypes "github.com/aws/aws-sdk-go-v2/service/sso/types"
	"github.com/aws/aws-sdk-go-v2/service/ssooidc"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/aws/aws-sdk-go-v2/service/sts/types"
	"github.com/schollz/progressbar/v3"
	"github.com/spf13/cobra"
	"gopkg.in/ini.v1"
)

type Config struct {
	Profile  string
	TagName  string
	Command  string
	Document string
}

type AWSClient struct {
	Config       aws.Config
	SSOClient    *sso.Client
	EC2Client    *ec2.Client
	SSMClient    *ssm.Client
	STSClient    *sts.Client
	SSOIDCClient *ssooidc.Client
}

type App struct {
	config    *Config
	awsClient *AWSClient
}

// Version is set during build using ldflags
var version = "dev"

func NewAWSClient(cfg aws.Config) *AWSClient {
	return &AWSClient{
		Config:       cfg,
		SSOClient:    sso.NewFromConfig(cfg),
		EC2Client:    ec2.NewFromConfig(cfg),
		SSMClient:    ssm.NewFromConfig(cfg),
		STSClient:    sts.NewFromConfig(cfg),
		SSOIDCClient: ssooidc.NewFromConfig(cfg),
	}
}

func NewApp(config *Config) *App {
	return &App{
		config: config,
	}
}

func main() {
	var (
		profile     string
		command     string
		document    string
		showVersion bool
	)

	rootCmd := &cobra.Command{
		Use:   "aws-ssm-connect [instance-tag-name]",
		Short: "Connect to EC2 instances via AWS SSM with automatic SSO authentication",
		Args:  cobra.ExactArgs(1),
		Run: func(cmd *cobra.Command, args []string) {
			if showVersion {
				fmt.Printf("aws-ssm-connect version %s\n", version)
				return
			}

			config := &Config{
				Profile:  profile,
				TagName:  args[0],
				Command:  command,
				Document: document,
			}

			app := NewApp(config)
			if err := app.Run(); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				os.Exit(1)
			}
		},
	}

	rootCmd.Flags().StringVarP(&profile, "profile", "p", "default", "AWS profile name to use")
	rootCmd.Flags().StringVarP(&command, "command", "c", "bash", "Command to execute on the instance")
	rootCmd.Flags().StringVarP(&document, "document", "d", "AWS-StartInteractiveCommand", "SSM document name to use")
	rootCmd.Flags().BoolVarP(&showVersion, "version", "v", false, "Show version information")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func (a *App) Run() error {
	ctx := context.Background()

	if err := a.createAWSSession(ctx); err != nil {
		return fmt.Errorf("failed to create AWS session: %w", err)
	}

	instance, err := a.findInstance(ctx, a.config.TagName)
	if err != nil {
		return fmt.Errorf("failed to find instance: %w", err)
	}

	cmd := exec.Command("aws", "ssm", "start-session",
		"--target", *instance.InstanceId,
		"--document-name", a.config.Document,
		"--region", a.awsClient.Config.Region,
	)

	if a.config.Profile != "default" {
		cmd.Args = append(cmd.Args, "--profile", a.config.Profile)
	}

	if a.config.Command != "" && a.config.Command != "bash" {
		cmd.Args = append(cmd.Args, "--parameters", fmt.Sprintf("command='%s'", a.config.Command))
	}

	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func (a *App) startSSOLogin(ctx context.Context, startURL, region string) (string, error) {
	registerClientOutput, err := a.awsClient.SSOIDCClient.RegisterClient(ctx, &ssooidc.RegisterClientInput{
		ClientName: aws.String("aws-ssm-connect"),
		ClientType: aws.String("public"),
		Scopes:     []string{"sso:account:access"},
	})
	if err != nil {
		return "", fmt.Errorf("failed to register client: %w", err)
	}

	startDeviceAuthOutput, err := a.awsClient.SSOIDCClient.StartDeviceAuthorization(ctx, &ssooidc.StartDeviceAuthorizationInput{
		ClientId:     registerClientOutput.ClientId,
		ClientSecret: registerClientOutput.ClientSecret,
		StartUrl:     aws.String(startURL),
	})
	if err != nil {
		return "", fmt.Errorf("failed to start device authorization: %w", err)
	}

	fmt.Printf("\nOpening browser for authentication...\n")
	fmt.Printf("If the browser doesn't open automatically, please visit: %s\n", *startDeviceAuthOutput.VerificationUriComplete)

	if err := openBrowser(*startDeviceAuthOutput.VerificationUriComplete); err != nil {
		fmt.Printf("Failed to open browser: %v\n", err)
	}

	fmt.Println("\nWaiting for authentication...")
	bar := progressbar.Default(60)
	for i := 0; i < 60; i++ {
		tokenOutput, err := a.awsClient.SSOIDCClient.CreateToken(ctx, &ssooidc.CreateTokenInput{
			ClientId:     registerClientOutput.ClientId,
			ClientSecret: registerClientOutput.ClientSecret,
			DeviceCode:   startDeviceAuthOutput.DeviceCode,
			GrantType:    aws.String("urn:ietf:params:oauth:grant-type:device_code"),
		})
		if err == nil {
			bar.Finish()
			return *tokenOutput.AccessToken, nil
		}
		time.Sleep(time.Second)
		bar.Add(1)
	}

	return "", fmt.Errorf("authentication timed out")
}

func (a *App) selectAccountAndRole(ctx context.Context, accessToken string) (string, string, error) {
	accountsOutput, err := a.awsClient.SSOClient.ListAccounts(ctx, &sso.ListAccountsInput{
		AccessToken: &accessToken,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to list accounts: %w", err)
	}

	if len(accountsOutput.AccountList) == 0 {
		return "", "", fmt.Errorf("no accounts found")
	}

	var selectedAccount *ssoTypes.AccountInfo
	if len(accountsOutput.AccountList) == 1 {
		selectedAccount = &accountsOutput.AccountList[0]
	} else {
		fmt.Println("\nAvailable accounts:")
		for i, account := range accountsOutput.AccountList {
			fmt.Printf("%d. %s (%s)\n", i+1, *account.AccountName, *account.AccountId)
		}

		reader := bufio.NewReader(os.Stdin)
		fmt.Print("\nSelect account number: ")
		accountNumStr, err := reader.ReadString('\n')
		if err != nil {
			return "", "", fmt.Errorf("failed to read account selection: %w", err)
		}
		accountNum, err := strconv.Atoi(strings.TrimSpace(accountNumStr))
		if err != nil || accountNum < 1 || accountNum > len(accountsOutput.AccountList) {
			return "", "", fmt.Errorf("invalid account selection")
		}
		selectedAccount = &accountsOutput.AccountList[accountNum-1]
	}

	rolesOutput, err := a.awsClient.SSOClient.ListAccountRoles(ctx, &sso.ListAccountRolesInput{
		AccessToken: &accessToken,
		AccountId:   selectedAccount.AccountId,
	})
	if err != nil {
		return "", "", fmt.Errorf("failed to list roles: %w", err)
	}

	if len(rolesOutput.RoleList) == 0 {
		return "", "", fmt.Errorf("no roles found for account %s", *selectedAccount.AccountName)
	}

	var selectedRole *ssoTypes.RoleInfo
	if len(rolesOutput.RoleList) == 1 {
		selectedRole = &rolesOutput.RoleList[0]
	} else {
		fmt.Println("\nAvailable roles:")
		for i, role := range rolesOutput.RoleList {
			fmt.Printf("%d. %s\n", i+1, *role.RoleName)
		}

		reader := bufio.NewReader(os.Stdin)
		fmt.Print("\nSelect role number: ")
		roleNumStr, err := reader.ReadString('\n')
		if err != nil {
			return "", "", fmt.Errorf("failed to read role selection: %w", err)
		}
		roleNum, err := strconv.Atoi(strings.TrimSpace(roleNumStr))
		if err != nil || roleNum < 1 || roleNum > len(rolesOutput.RoleList) {
			return "", "", fmt.Errorf("invalid role selection")
		}
		selectedRole = &rolesOutput.RoleList[roleNum-1]
	}

	return *selectedAccount.AccountId, *selectedRole.RoleName, nil
}

func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start()
}

func (a *App) findInstance(ctx context.Context, instanceName string) (*ec2Types.Instance, error) {
	input := &ec2.DescribeInstancesInput{
		Filters: []ec2Types.Filter{
			{
				Name:   aws.String("tag:Name"),
				Values: []string{instanceName},
			},
		},
	}

	result, err := a.awsClient.EC2Client.DescribeInstances(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to describe instances: %w", err)
	}

	for _, reservation := range result.Reservations {
		for _, instance := range reservation.Instances {
			if instance.State.Name == ec2Types.InstanceStateNameRunning {
				return &instance, nil
			}
		}
	}

	return nil, fmt.Errorf("no running instance found with name %s", instanceName)
}

func (a *App) getSSOCredentials(ctx context.Context, accessToken, accountID, roleName string) (*types.Credentials, error) {
	output, err := a.awsClient.SSOClient.GetRoleCredentials(ctx, &sso.GetRoleCredentialsInput{
		AccessToken: &accessToken,
		AccountId:   &accountID,
		RoleName:    &roleName,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get role credentials: %w", err)
	}

	expiration := time.Unix(output.RoleCredentials.Expiration, 0)
	return &types.Credentials{
		AccessKeyId:     output.RoleCredentials.AccessKeyId,
		SecretAccessKey: output.RoleCredentials.SecretAccessKey,
		SessionToken:    output.RoleCredentials.SessionToken,
		Expiration:      &expiration,
	}, nil
}

func (a *App) createAWSSession(ctx context.Context) error {
	cfg, err := config.LoadDefaultConfig(ctx,
		config.WithSharedConfigProfile(a.config.Profile),
	)
	if err != nil {
		return fmt.Errorf("failed to load AWS config: %w", err)
	}

	a.awsClient = NewAWSClient(cfg)
	_, err = a.awsClient.STSClient.GetCallerIdentity(ctx, &sts.GetCallerIdentityInput{})
	if err == nil {
		return nil
	}

	configPath := os.ExpandEnv("$HOME/.aws/config")
	awsConfig, err := ini.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load AWS config file: %w", err)
	}

	profileSection := awsConfig.Section(fmt.Sprintf("profile %s", a.config.Profile))
	if profileSection == nil {
		return fmt.Errorf("profile %s not found in AWS config", a.config.Profile)
	}

	ssoStartURL := profileSection.Key("sso_start_url").String()
	ssoRegion := profileSection.Key("sso_region").String()

	if ssoStartURL == "" || ssoRegion == "" {
		return fmt.Errorf("SSO configuration not found in profile %s", a.config.Profile)
	}

	accessToken, err := a.startSSOLogin(ctx, ssoStartURL, ssoRegion)
	if err != nil {
		return fmt.Errorf("failed to start SSO login: %w", err)
	}

	accountID, roleName, err := a.selectAccountAndRole(ctx, accessToken)
	if err != nil {
		return fmt.Errorf("failed to select account and role: %w", err)
	}

	creds, err := a.getSSOCredentials(ctx, accessToken, accountID, roleName)
	if err != nil {
		return fmt.Errorf("failed to get SSO credentials: %w", err)
	}

	cfg, err = config.LoadDefaultConfig(ctx,
		config.WithRegion(ssoRegion),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
			*creds.AccessKeyId,
			*creds.SecretAccessKey,
			*creds.SessionToken,
		)),
	)
	if err != nil {
		return fmt.Errorf("failed to create AWS config: %w", err)
	}

	a.awsClient = NewAWSClient(cfg)
	return nil
}

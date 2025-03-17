# AWS SSM Connect

A CLI tool to easily connect to EC2 instances via AWS SSM with automatic SSO authentication.

## Installation

### Quick Install (macOS and Linux)

```bash
curl -fsSL https://raw.githubusercontent.com/shushpan/aws-ssm-connect/main/install.sh | bash
```

### Using Homebrew (macOS)

```bash
brew install shushpan/tap/aws-ssm-connect
```

Alternatively, you can add the tap first and then install:

```bash
brew tap shushpan/tap
brew install aws-ssm-connect
```

### Using Go

```bash
go install github.com/shushpan/aws-ssm-connect@latest
```

### From Binary Releases

Download the latest binary for your platform from the [Releases](https://github.com/shushpan/aws-ssm-connect/releases) page.

### Building from Source

```bash
git clone https://github.com/shushpan/aws-ssm-connect.git
cd aws-ssm-connect
make build
```

## Prerequisites

- **AWS CLI** (v2+) - [Installation Guide](https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html)
- **Session Manager Plugin** - [Installation Guide](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html) or `brew install session-manager-plugin` on macOS

### AWS Configuration

For SSO profiles, ensure your AWS config includes:
```ini
[profile your-profile]
sso_start_url = https://your-sso-portal.awsapps.com/start
sso_region = us-east-1
region = us-east-1
```

### Required Permissions

- Your IAM user/role needs: `ssm:StartSession`, `ssm:TerminateSession`, `ec2:DescribeInstances`
- EC2 instances must have:
  - SSM Agent installed
  - IAM role with `AmazonSSMManagedInstanceCore` policy
  - "Name" tag for identification

## Usage

```bash
# Basic usage - connect to an instance by tag name
aws-ssm-connect my-instance-name

# Specify AWS profile
aws-ssm-connect my-instance-name -p my-profile

# Run a specific command instead of bash
aws-ssm-connect my-instance-name -c "cd /var/www && ls -la"

# Use a different SSM document
aws-ssm-connect my-instance-name -d MyCustomDocument

# Show version information
aws-ssm-connect --version
```

### Command Line Options

- `-p, --profile`: AWS profile name to use (default: "default")
- `-c, --command`: Command to execute on the instance (default: "bash")
- `-d, --document`: SSM document name to use (default: "AWS-StartInteractiveCommand")
- `-v, --version`: Show version information
- `-h, --help`: Display help information

You can omit the `-c` and `-d` flags, and the tool will use the default values.

### AWS Profile Selection

If you don't specify a profile with the `-p` flag, the tool will:

1. Detect all available AWS profiles from your `~/.aws/config` and `~/.aws/credentials` files
2. Display a list of available profiles
3. Prompt you to select a profile by number
4. Use the selected profile for the connection

This makes it easy to switch between different AWS accounts without having to remember profile names.

## AWS SSO Support

The tool automatically handles AWS SSO authentication if your profile is configured to use SSO. It will:

1. Open a browser for authentication
2. Allow you to select an account and role if multiple are available
3. Use the credentials to establish the SSM session

## Development

### Requirements

- Go 1.18 or higher
- AWS SDK for Go v2

### Development Commands

```bash
# Build the binary
make build

# Install locally
make install

# Clean build artifacts
make clean

# Create a release (requires GoReleaser)
make release
```

## License

MIT

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request. See [CONTRIBUTING.md](CONTRIBUTING.md) for more details. 
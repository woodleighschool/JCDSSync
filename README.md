# JCDSSync

A Python application that synchronises JCDS (Jamf Cloud Distribution Service) packages to a local folder via the Jamf Pro API.

## Features

-   Authenticates with Jamf Pro API using OAuth2 client credentials
-   Downloads packages from JCDS to a local directory
-   Verifies file integrity using MD5 checksums
-   Removes outdated files that are no longer in JCDS
-   Supports scheduled synchronisation using cron expressions
-   Configurable logging levels
-   Can run as a one-time sync or scheduled background process

## Installation

### Option 1: From GitHub Release (Recommended for Docker)

Download the wheel file from the [latest release](https://github.com/woodleighschool/JCDSSync/releases) and install:

```bash
pip3 install jcdssync-*.whl
```

### Option 2: From Source

1. Clone the repository:

```bash
git clone https://github.com/woodleighschool/JCDSSync.git
cd JCDSSync
```

2. Install dependencies:

```bash
pip3 install -r requirements.txt
```

3. Or install the package:

```bash
pip3 install .
```

### Option 3: Direct from GitHub

```bash
# Install latest release
pip3 install git+https://github.com/woodleighschool/JCDSSync.git

# Install specific version
pip3 install git+https://github.com/woodleighschool/JCDSSync.git@v1.0.0
```

## Configuration

The application uses environment variables for configuration:

| Variable             | Description                                         | Required | Default                         |
| -------------------- | --------------------------------------------------- | -------- | ------------------------------- |
| `JAMF_URL`           | Your Jamf Pro server URL                            | Yes      | -                               |
| `JAMF_CLIENT_ID`     | OAuth2 client ID                                    | Yes      | -                               |
| `JAMF_CLIENT_SECRET` | OAuth2 client secret                                | Yes      | -                               |
| `SYNC_NOW`           | Run sync immediately (`true`/`false`)               | No       | `false`                         |
| `SYNC_SCHEDULE`      | Cron schedule for sync                              | No       | `0 0 * * *` (daily at midnight) |
| `LOG_LEVEL`          | Logging level (`DEBUG`, `INFO`, `WARNING`, `ERROR`) | No       | `INFO`                          |

## Usage

### Command Line

```bash
# Run once
JAMF_URL="https://your-jamf-server.com" \
JAMF_CLIENT_ID="your-client-id" \
JAMF_CLIENT_SECRET="your-client-secret" \
SYNC_NOW=true \
python3 jamfsync.py

# Run with scheduled sync (daily at 2 AM)
JAMF_URL="https://your-jamf-server.com" \
JAMF_CLIENT_ID="your-client-id" \
JAMF_CLIENT_SECRET="your-client-secret" \
SYNC_SCHEDULE="0 2 * * *" \
python3 jamfsync.py
```

### As an Installed Package

```bash
# After pip install
JAMF_URL="https://your-jamf-server.com" \
JAMF_CLIENT_ID="your-client-id" \
JAMF_CLIENT_SECRET="your-client-secret" \
jcdssync
```

## Jamf Pro API Setup

1. In Jamf Pro, go to Settings > System Settings > API Roles and Privileges
2. Create a new role with the following privileges:
    - Jamf Content Distribution Service: Read
    - Packages: Read
3. Go to Settings > System Settings > API Integrations and Credentials
4. Create a new API Client with the role created above
5. Note the Client ID and Client Secret for configuration

## Local Package Storage

By default, packages are downloaded to `/packages`. Make sure this directory exists and has appropriate permissions:

```bash
mkdir -p /packages
chmod 755 /packages

```

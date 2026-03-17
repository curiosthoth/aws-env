AWS-Env
====

`aws-env` resolves environment variable values from AWS Secrets Manager or AWS Systems Manager Parameter Store and can either:

- act as an entrypoint wrapper that replaces itself with your target process, or
- read `KEY=value` pairs from stdin and emit resolved values to stdout.

It uses the default AWS SDK for Go v2 configuration chain. For local testing, the repo includes a `moto`-backed Docker Compose setup and a `Makefile`.

## Supported Schemes

- `secretsmanager://name`
- `ssm://name`

Both schemes support:

- raw value lookup
- nested JSON key lookup
- a configurable delimiter between the remote name and JSON key segments

The default delimiter is `##`.

## Reference Syntax

### Raw values

If the remote value is a plain string, reference it without any JSON path:

```text
API_TOKEN=secretsmanager://prod/api-token
DB_PASSWORD=ssm:///prod/db/password
```

### JSON values

If the remote value is JSON, append one or more key segments separated by the configured delimiter:

```text
DB_USER=secretsmanager://prod/app-config##database##username
DB_PASS=ssm:///prod/app-config##database##password
```

That resolves against payloads like:

```json
{
  "database": {
    "username": "app-user",
    "password": "app-pass"
  }
}
```

Internally, `aws-env` converts the key segments into a dotted lookup path such as `database.username`.

### Custom delimiter

Use `-d` to override the delimiter when your secret or parameter naming format conflicts with `##`:

```text
CONFIG=secretsmanager://prod/app-config::database::username
```

```bash
aws-env -p -d '::'
```

## Usage

### Entrypoint mode

Entrypoint mode is the default. `aws-env` scans the current environment, resolves supported references, updates the matching environment variables, and then `exec`s the command you pass to it.

```bash
NAME=secretsmanager://my/name aws-env printenv NAME
```

With nested JSON lookup:

```bash
APP_USER=ssm:///prod/app-config##database##username \
APP_PASS=ssm:///prod/app-config##database##password \
aws-env env
```

### Pipe mode

Use `-p` to read lines from stdin and emit resolved values to stdout.

```bash
echo 'NAME=secretsmanager://my/name' | aws-env -p
```

Output:

```text
NAME=Aya Brea
```

Pipe mode is useful with `eval`:

```bash
eval "$(echo 'NAME=secretsmanager://my/name' | aws-env -p)"
echo "$NAME"
```

Export-friendly output:

```bash
echo 'TOKEN=ssm:///prod/api/token' | aws-env -p -e
```

Output:

```text
export TOKEN=super-secret-token
```

Associative-array output for Bash:

```bash
printf '%s\n' \
  'USER=secretsmanager://prod/app-config##database##username' \
  'PASS=secretsmanager://prod/app-config##database##password' \
  | aws-env -p -a SECRETS
```

Output:

```text
SECRETS[USER]=app-user
SECRETS[PASS]=app-pass
```

## Flags

- `-p`: enable pipe mode
- `-a <name>`: emit Bash associative array assignments in pipe mode
- `-e`: prefix pipe-mode output with `export `
- `-s`: suppress warnings and lookup errors
- `-d <delimiter>`: override the JSON key delimiter; defaults to `##`

## AWS Configuration

`aws-env` uses the AWS SDK default config chain, including standard environment variables, shared config files, IAM roles, and web identity credentials.

For local moto-based testing, the test flow sets:

```text
AWS_ACCESS_KEY_ID=test
AWS_SECRET_ACCESS_KEY=test
AWS_REGION=us-east-1
AWS_ENDPOINT_URL=http://127.0.0.1:5000
```

`AWS_ENDPOINT_URL` is also honored by the runtime client constructor, which lets you point both Secrets Manager and SSM calls at a local emulator.

## Development

Run the full test suite against moto:

```bash
make test
```

Build all supported release targets:

```bash
make build
```

Generated binaries:

- `bin/aws-env-linux-amd64`
- `bin/aws-env-linux-arm64`
- `bin/aws-env-darwin-arm64`

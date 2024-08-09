AWS-Env
====

A simple tool for substituting environment variables with values from AWS Secrets Manager.
It checks the environment variables for values starting with `secretsmanager://<key>` and fetches the value from AWS
Secrets Manager by `<key>`.

This tool operates in two modes.

By default, it functions as an entrypoint for a container (Entrypoint Mode).
Typically, you should set it as the Entrypoint for a Docker container, where it will Exec the actual command with
substituted environment variables. Note that aws-env is not meant to serve as the init process (1); instead, the
launched executable will replace the current aws-env process. For an init process, consider using aws-env with
[tini](https://github.com/krallin/tini).

When the -p flag is used, it runs in Pipe Mode. Under this mode, it reads from STDIN and output to STDOUT in the
format "KEY=VALUE" by default.
If you use the -a ARRAY_NAME flag, the output will be formatted as an associative array, like `ARRAY_NAME[KEY]=VALUE`.

You can also provide a JMESPath expression, separated by "##" (double pound sign), to retrieve a specific value from the
JSON-formatted secret.

E.g., for `secretsmanager://my/secret##SecretString` it will fetch the value of the key `SecretString` from the secret
like `{"SecretString": "Hello, World!"}`.

This tools uses the default AWS credentials chain to authenticate with AWS. It will also try to cache the fetched values
to avoid unnecessary calls to AWS.

## Examples
Assume that we saved a secret with name `my/name` in AWS SecretsManager with the value `Aya Brea`.

### Example 1 - Pipe mode


```bash
$ eval $(echo "NAME=secretsmanager://my/name" | aws-env -p)
$ echo "Hello, my name is $NAME"
>>> Hello, my name is Aya Brea
```

You can see that the environment variable `NAME` was substituted with the value `Aya Brea`.

### Example 2 - Entrypoint mode

We have a Dockerfile with the following content:

```Dockerfile
FROM alpine:latest

ENTRYPOINT ["aws-env"]
```

We build the image and run it with the following command:

```bash
$ docker run --rm -e NAME=secretsmanager://my/name my-image echo "Hello, my name is $NAME"
>>> Hello, my name is Aya Brea
```


## Cross Building for Linux

```shell
$ GOOS=linux CGO_ENABLED=0 GOARCH=amd64 go build -a -gcflags=all="-l -B" -ldflags="-w -s"
```
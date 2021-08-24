# Minio.io UI

This is not tends to be full replacement of embedded Minio UI or [Mini console](https://github.com/minio/console).

The project tries to provide minimal administrative web console to manage minio instance.

Features:

* single, server-side credential (no login screen) - supposed to be behind SSO or Oauth2
* can be deployed under any subpath (unlike minio UI/minio console)
* proxies all requests to minio by server - no need to direct access to minio instance (good for docker-compose or internal clusters).
* no javascript, no external dependencies

## Installation

### Docker

**Docker** installation designed to be primary. Image runs under unprivileged user.

Image: `ghcr.io/reddec/miniconsole`

Example compose file

```yaml
version: '3'
services:
  minio:
    image: minio/minio
    command:
      - server
      - /data
  ui:
    image: ghcr.io/reddec/miniconsole
    ports:
      - '9001:9001'
```

Supported environment variables:

* `MINIO_ENDPOINT` (default: `minio:9000`) - address of minio
* `MINIO_KEY_ID` (default: `minioadmin`) - minio key id
* `MINO_ACCESS_KEY` (default: `minioadmin`) - minio access key
* `MINIO_SSL` (default: `false`) - use SSL connection to minio
* `BIND` (default: `:9001`) - server binding
* `MAX_OBJECTS` (default: `1024`) - maximum number of objects info could be returned during listing

### From source

Requirements:
* go 1.17+

Build:
- clone project
- build by `go build -ldflags="-w -s" -trimpath ./cmd/...`

Run:
- `./miniconsole --help`

> Default values for configuration could be different from in docker.

## Screenshots

![Screenshot_20210825_014546](https://user-images.githubusercontent.com/6597086/130664738-b09ab1af-c604-47b0-9d41-5eeb4eba3008.png)
![Screenshot_20210825_014606](https://user-images.githubusercontent.com/6597086/130664745-9399b6ac-5769-436f-9e6a-4f254d1e6588.png)
![Screenshot_20210825_014623](https://user-images.githubusercontent.com/6597086/130664746-d968059a-f8d0-44ad-b4de-f344c7763138.png)

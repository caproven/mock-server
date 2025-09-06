# mock-server

Simple HTTP mock server, prioritizing configurability and ease of use.

## Installation

With the Go toolchain installed, run

```bash
go install github.com/caproven/mock-server@latest

mock-server -config <config_file>
```

A Docker image can also be built locally. Note that with this approach, the config file must be mounted within the container and specified as an argument to the CLI.

```bash
# With the repo cloned locally
docker build . -t mock-server:latest

docker run \
  -p 8080:8080 \
  --mount type=bind,src=<path_to_config>,dst=/conf/config.yaml \
  mock-server:latest -config /conf/config.yaml
```

## Configuration

The mock server requires a config file defining REST endpoints to serve. There are several response "strategies" available, configuring behavior of an endpoint as it is hit multiple times.

The core type is a "response", which directly describes the HTTP response received when hitting an endpoint. This type is embedded in all response strategies so common fields in one will work in the rest. A response looks something like

```yaml
# Would be nested somewhere in your config

# HTTP status code
status: 200 # defaults to 200
# HTTP response headers
headers:
  <key1>: <val>
  <key2>: <val>
body:
  # String value for the HTTP response body
  literal: |
    This will be in the resp body.
    Isn't that neat?
```

### Static Responses

Static responses do not change - the same response is returned every time.

```yaml
endpoints:
  - path: /api/v1/users/12
    method: GET
    response:
      static:
        status: 200
        headers:
          content-type: application/json
        body:
          literal: |-
            {
              "id": 12,
              "firstName": "Abraham",
              "lastName": "Lincoln"
            }
```

### Sequence of Responses

Responses can be sequenced, iterating upon each request to the endpoint.

```yaml
endpoints:
  - path: /doc-upload
    method: POST
    response:
      sequence:
        endBehavior: loop # defaults to 'loop', one of [loop, repeatLast]
        responses:
          - count: 4 # defaults to 1
            response:
              status: 200
          - count: 1
            response:
              status: 429
```

This example returns a 200 status code for the first 4 POST calls to `/doc-upload`, but on the fifth call it will return a 429 status code. The `count` field conveniently lets you repeat entries without duplicating them in the config.

Also take note of the `endBehavior` field - it controls behavior of the sequence once the endpoint has been called enough times that the sequence is exhausted. The default value, 'loop', will cause further calls to "reset" back to the beginning of the sequence. Another value 'repeatLast' instructs the sequence to repeat its last value indefinitely once the sequence is exhausted.

### Weighted Random Responses

An element of randomization can be added to response behavior. With the weighted strategy, entries are randomly selected from all available options. Weights can be provided to control the likelihood of entries being selected. The weight values are summed and the chance of any given entry being selected is its weight divided by the total configured weights.

```yaml
endpoints:
  - path: /index.html
    method: GET
    response:
      weighted:
        - weight: 9 # defaults to 1
          response:
            body:
              literal: <p>Hello world!</p>
        - weight: 1
          response:
            status: 500
```

This example emulates a web server flaking. The `/index.html` path has a 90% chance of returning some HTML with a 200 status and a 10% chance of returning a 500 status.

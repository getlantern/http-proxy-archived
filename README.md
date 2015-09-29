# Lantern Chained Server in Go

## Run

First get dependencies:
```
go get -t
```

Compile and run:
```
go build && ./chained-server
```

Or run directly:

```
go run main.go
```

## Test

Direct proxying:

```
curl -vLkx localhost:8080 http://bing.com --proxy-header "X-Lantern-Auth-Token: 111"
curl -vLkx localhost:8080 https://bing.com --proxy-header "X-Lantern-Auth-Token: 111"
```

Using HTTP connect:

```
curl -vLkxp localhost:8080 http://bing.com --proxy-header "X-Lantern-Auth-Token: 111"
```

Without the header, it will respond `404 Not Found`.

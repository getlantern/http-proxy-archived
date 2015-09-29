## Run
go get -t
go build && ./chained-server

## Test

> curl -vLkx localhost:8080 http://bing.com --proxy-header "X-Lantern-Auth-Token: 111"
> curl -vLkx localhost:8080 https://bing.com --proxy-header "X-Lantern-Auth-Token: 111"

Without the header, it will respond `404 Not Found`.

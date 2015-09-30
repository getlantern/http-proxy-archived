# Lantern Chained Server in Go

## Run

First get dependencies:
```
go get -t
```
Then run with:

```
./run-server.sh
```

## Test

Direct proxying:

```
curl -kvx localhost:8080 http://www.google.com/humans.txt --proxy-header "X-Lantern-Auth-Token: 111"
curl -kvx localhost:8080 https://www.google.com/humans.txt --proxy-header "X-Lantern-Auth-Token: 111"
```

Using HTTP connect:

```
curl -kpvx localhost:8080 http://www.google.com/humans.txt --proxy-header "X-Lantern-Auth-Token: 111"
curl -kpvx localhost:8080 https://www.google.com/humans.txt --proxy-header "X-Lantern-Auth-Token: 111"
```

*Keep in mind that cURL doesn't support tunneling through an HTTPS proxy, so if you use the -https option you have to use other tools for testing*

Without the header, it will respond `404 Not Found`.

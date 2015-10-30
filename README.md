# Lantern Chained Server in Go

## Run

First get dependencies:
```
go get -t
```
Then run with:

```
test/run-server.sh
```

## Test

*Keep in mind that cURL doesn't support tunneling through an HTTPS proxy, so if you use the -https option you have to use other tools for testing.

Without the header, it will respond `404 Not Found`. In order to avoid this and test it as a normal proxy, use the `-disableFilters` flag.*

You can either use the proxy as a regular HTTP proxy or with Lantern-specific extensions.

### Testing as a regular HTTP proxy

Run the server as follows:

```
test/run-server.sh -disablefilters
```

Test direct proxying with cURL:

```
curl -kvx localhost:8080 http://www.google.com/humans.txt
curl -kvx localhost:8080 https://www.google.com/humans.txt
```

Test HTTP connect with cURL:

```
curl -kpvx localhost:8080 http://www.google.com/humans.txt
curl -kpvx localhost:8080 https://www.google.com/humans.txt
```

### Testing with Lantern extensions

Run the server with:

```
test/run-server.sh -https -token=<your-token>
```

You have two options to test it: the Lantern client or [checkfallbacks](https://github.com/getlantern/lantern/tree/valencia/src/github.com/getlantern/checkfallbacks).

Keep in mind that they will need to send some headers in order to avoid receiving 404 messages (the chained server response if you aren't providing them).

Currently, the only header you need to add is `X-Lantern-Device-Id`.

If you are using checkfallbacks, make sure that both the certificate and the token are correct.  A 404 will be the reply otherwise.  Running the server with `-debug` may help you troubleshooting those scenarios.


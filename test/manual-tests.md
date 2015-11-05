# Manual tests

For these manual tests, you'll need 3 servers (*client*, *proxy*, *origin*), preferably in the same network so you can reach the highest possible transfer rates and smaller latencies.  Any suggestion to improve these tests is welcome.



## Monitoring strategies and tools

### General

Look at DigitalOcean's graph data (Disk, CPU, Bandwidth).

### Memory footprint monitoring

Monitor memory with

```
watch -n 0.5 'pmap <process-pid> | tail -n 1'
```

### Connection monitoring

Monitor transfers per connection with

```
nethogs
```



## Test large file transmission

### Download a large file with fast origin link and fast client link

#### Download with unbounded transfer rates

In client machine:

```
wget -e use_proxy=yes -e http_proxy=<proxy-addr>:8080 http://releases.ubuntu.com/15.10/ubuntu-15.10-desktop-amd64.iso
```

In proxy machine: use memory footprint monitoring and connection monitoring.

#### Download with client rate limit

In client machine:

```
wget --limit-rate=20k -e use_proxy=yes -e http_proxy=<proxy-addr>:8080 http://releases.ubuntu.com/15.10/ubuntu-15.10-desktop-amd64.iso
```

In proxy machine: use memory footprint monitoring and connection monitoring.

#### Download with origin rate limit

<TODO>

#### Upload with unbounded transfer rates

<TODO>

#### Upload with client rate limit

<TODO>

#### Upload with origin rate limit

<TODO>

#### Tests with multiple files simultaneously

Force multiple connections to be opened, by downloading multiple times. Using `wget` will rename automatically, so many independent simultaneous downloads can be performed.

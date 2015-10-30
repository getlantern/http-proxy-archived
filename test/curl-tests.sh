#!/usr/bin/env sh

echo "Testing Direct Proxying HTTP target"
curl -kvx localhost:8080 http://www.google.com/humans.txt --proxy-header "X-Lantern-Auth-Token: 111" --proxy-header "X-Lantern-UID: 1234-1234-1234-1234-1234-1234"

echo "Testing CONNECT HTTP target"
curl -kpvx localhost:8080 http://www.google.com/humans.txt --proxy-header "X-Lantern-Auth-Token: 111" --proxy-header "X-Lantern-UID: 5678-5678-5678-5678-5678-5678"

echo "Testing CONNECT HTTPS target"
curl -kpvx localhost:8080 https://www.google.com/humans.txt --proxy-header "X-Lantern-Auth-Token: 111" --proxy-header "X-Lantern-UID: 1515-1515-1515-1515-1515-1515"

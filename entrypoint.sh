#!/bin/bash
set -e

rpk redpanda start \
  --smp 1 \
  --memory 700MiB \
  --reserve-memory 0MiB \
  --overprovisioned \
  --check=false &

# Wait a bit for Redpanda to be up
sleep 5

/usr/local/bin/boggle-backend &  
/usr/local/bin/boggle-kafka &  

wait

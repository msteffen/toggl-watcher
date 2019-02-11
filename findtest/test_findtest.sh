#!/bin/bash

set -e

go build .

rm -rf ./dir
mkdir ./dir

./findtest ./dir &
sleep 2

cmds=(
  "mkdir dir/a"
  "mkdir dir/b"
  "mkdir dir/c"
  "echo \"test\" >dir/a/test.txt"
)
set -x
for c in "${cmds[@]}"; do
  eval "${c}"
  sleep 1
done

sleep 2
killall findtest

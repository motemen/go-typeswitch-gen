#!/bin/sh

set -e

base=$(cd $(dirname $0) && pwd)

cd $base

for e in *; do
    if [ ! -d $e ]; then
        continue
    fi

    echo "# $e"

    cd $e

    git diff --quiet .

    go generate
    git --no-pager diff .

    if [ $(go list -f '{{.Name}}' .) = "main" ]; then
        go run $(go list -f '{{join .GoFiles " "}}' .)
    fi

    git checkout .

    cd $base
done

#!/bin/bash
# Script to create required folder structure with .keep markers

set -e

# Remove existing conflicting dirs
rm -rf apps docs dashboard cmd runtime workers schemas tests deploy packages

# Create directory structure
mkdir -p apps/{website,docs,dashboard}
mkdir -p cmd/api
mkdir -p runtime/{translator,bridge}
mkdir -p workers/{python-adapter,go-adapter}
mkdir -p schemas/{openapi,ir}
mkdir -p tests/{conformance,soak}
mkdir -p deploy/{compose,helm}
mkdir -p packages/{ui-tokens,tracking}
mkdir -p .github/workflows

# Add .keep files
touch apps/website/.keep
touch apps/docs/.keep
touch apps/dashboard/.keep
touch cmd/api/.keep
touch runtime/translator/.keep
touch runtime/bridge/.keep
touch workers/python-adapter/.keep
touch workers/go-adapter/.keep
touch schemas/openapi/.keep
touch schemas/ir/.keep
touch tests/conformance/.keep
touch tests/soak/.keep
touch deploy/compose/.keep
touch deploy/helm/.keep
touch packages/ui-tokens/.keep
touch packages/tracking/.keep

echo "Folder structure initialized."

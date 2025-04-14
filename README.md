# Model Runner

The backend library for the
[Docker Model Runner](https://docs.docker.com/desktop/features/model-runner/).

## Overview

> [!NOTE]
> This package is still under rapid development and its APIs should not be
> considered stable.

This package supports the Docker Model Runner in Docker Desktop (in conjunction
with [Model Distribution](https://github.com/docker/model-distribution) and the
[Model CLI](https://github.com/docker/model-cli)). It includes a `main.go` that
mimics its integration with Docker Desktop and allows the package to be run in a
standalone mode.

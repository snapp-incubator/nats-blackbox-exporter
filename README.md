<h1 align="center"> NATS Blackbox Exporter </h1>

<p align="center">
    <img src="./.github/assets/image.png" height="250px">
</p>

<p align="center">
    <img alt="GitHub Workflow Status" src="https://img.shields.io/github/actions/workflow/status/snapp-incubator/nats-blackbox-exporter/ci.yaml?logo=github&style=for-the-badge">
    <img alt="Codecov" src="https://img.shields.io/codecov/c/github/snapp-incubator/nats-blackbox-exporter?logo=codecov&style=for-the-badge">
    <img alt="GitHub repo size" src="https://img.shields.io/github/repo-size/snapp-incubator/nats-blackbox-exporter?logo=github&style=for-the-badge">
    <img alt="GitHub tag (with filter)" src="https://img.shields.io/github/v/tag/snapp-incubator/nats-blackbox-exporter?style=for-the-badge&logo=git">
    <img alt="GitHub go.mod Go version (subdirectory of monorepo)" src="https://img.shields.io/github/go-mod/go-version/snapp-incubator/nats-blackbox-exporter?style=for-the-badge&logo=go">
</p>

## Introduction

Each probe publishes a message over Jetstream NATS and then waits to receive it through a subscription. By measuring the time taken for this process, you can monitor the status of your NATS cluster. At Snapp!, we use this to detect the status of our NATS clusters from the client perspective.

You can track the following metrics:
- **Connection**: A `prometheus.CounterVec` that counts disconnections and connections.
- **Latency**: A `prometheus.Histogram` that measures the latency between publishing and subscribing.
- **SuccessCounter**: A `prometheus.CounterVec` that counts successful publishes and consumes.

This setup helps ensure that your NATS clusters are functioning optimally and provides insights into any issues from the client side.

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

## 🥁 Introduction
The NATS Blackbox Exporter is a tool designed to monitor and track key metrics related to NATS Jetstream. At Snapp!, we use this to detect the status of our NATS clusters from the client perspective.

This project uses the [github.com/nats-io/nats.go](https://pkg.go.dev/github.com/nats-io/nats.go) package to interact with NATS. Specifically, it utilizes the [JetStreamContext](https://pkg.go.dev/github.com/nats-io/nats.go#JetStreamContext) interface to enable JetStream messaging and stream management.

## 🗃️ What is a Blackbox Exporter?
A blackbox exporter is a type of software that monitors the availability and performance of services by performing remote checks, as opposed to whitebox monitoring which relies on internal metrics and logs. In this case, the blackbox exporter monitors NATS by measuring connection events, message latency, and success rates.

## 🚀 Getting Started
You can deploy and use NATS Blackbox Exporter using Docker images or by building from the source code.

### 1. Using Docker Image 📫
You can use pre-built Docker images from GitHub Container Registry (GHCR):
```bash
docker run -d -p 8080:8080 --name nats-blackbox-exporter -v ./config.yaml:/app/config.yaml:ro ghcr.io/snapp-incubator/nats-blackbox-exporter:<release-tag>
```
and then pass environment variables as needed.

### 2. Build from Source Code ⚙️
You can build the project from the source code:
```bash
go build -o nats-blackbox-exporter cmd/nats-blackbox-exporter/main.go
```

## ⏳ Usage
The exporter will generate Prometheus metrics on the port specified in the configuration file, accessible at the `/metrics` path. The key metrics tracked include:

- **Connection:** A `prometheus.CounterVec` that counts disconnections and connections.
- **Latency:** A `prometheus.Histogram` that measures the latency between publishing and subscribing.
- **SuccessCounter:** A `prometheus.CounterVec` that counts successful publishes and consumes.

## 🎨 Configuration
You can check the list of parameters with default values in the [config.example.yaml](./config.example.yaml) file. The NATS Blackbox Exporter can be configured in three ways:

1. **Environment Variables:**
   Set the necessary environment variables before running the exporter.

2. **Configuration File:**
   Use a `config.yaml` file to specify the configuration parameters.

3. **Default Values:**
   If neither environment variables nor a configuration file is provided, the exporter will use default values.

<!-- 
## ToDo(s) -->

# China-Dns

## Introduction
China-Dns is a custom DNS server implementation written in Go. It provides a reliable and configurable DNS resolution service, allowing users to specify primary and secondary DNS servers. The server also features a caching mechanism to enhance the performance of DNS queries.

## Features
- **Configurable DNS Servers:** Allows setting up primary and secondary DNS servers.
- **Custom Listen Address:** Ability to specify the listen address for the DNS server.
- **DNS Query Caching:** Includes a caching system to speed up DNS query responses.

## Installation
Ensure that Go is installed on your system. You can download Go from the official [Go website](https://golang.org/dl/).

Clone the China-Dns repository:

```bash
git clone https://github.com/alekseiapa/China-Dns.git
```

Navigate to the project directory:

```bash
cd China-Dns
```

Build the project:

```bash
go build
```

Usage

Run the server with the default settings using:

```bash
./China-Dns
```

For custom configurations, the following flags are available:

    -primarydns: Set the primary DNS server address (default "8.8.8.8").
    -secondarydns: Set the secondary DNS server address (default "8.8.4.4").
    -listenaddr: Set the DNS server's listen address (default "127.0.0.1:53").
    -cachesize: Specify the cache size (default 20480).

Example with custom settings:

```bash
./China-Dns -primarydns 1.1.1.1 -secondarydns 1.0.0.1 -listenaddr 0.0.0.0:53 -cachesize 4096
```

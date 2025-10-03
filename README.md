# Go URL Shortener API

A blazing-fast, lightweight URL shortening service built with Go and backed by Redis for efficient key-value storage.

## ✨ Features

* **Fast:** Built with Go for high performance and low latency.
* **Simple:** Minimal API endpoints for easy integration.
* **Persistent:** Uses Redis to store mappings for quick lookups.
* **Containerized:** Ready to run with Docker for consistent environments.

---

## 🚀 Getting Started

These instructions will get you a copy of the project up and running on your local machine for development and testing purposes.

### Prerequisites

You will need the following installed on your local machine:

* [Go (latest stable version)](https://go.dev/doc/install)
* [Redis Server](https://redis.io/download/) (running locally or accessible via a URL)
* (Optional, for containerization) [Docker](https://www.docker.com/get-started/)

### 🛠️ Local Setup

1.  **Clone the repository:**
    ```bash
    git clone https://github.com/girhty/sh-api.git
    cd sh-api
    ```

2.  **Create the environment file (`.env`):**
    Create a file named **`.env`** in the root directory and set the required environment variables:

    ```bash
    # The URL that will prefix the generated short code (e.g., http://localhost:8080 or [https://s.co](https://s.co))
    HOST=http://localhost:8443 # if the server is running locally else just the host name https://example.com

    # The URL for your Redis database (e.g., redis://localhost:6379)
    REDIS=redis://localhost:6379
    ```

3.  **Run the application:**
    ```bash
    go run main.go
    ```

    The API will now be running by default on port `8443` (or the port configured internally).

---

## 🐳 Docker Setup

For a more consistent and production-ready environment, you can run the application inside a Docker container.

1.  **Build the Docker Image:**
    Run this command in the project's root directory:
    ```bash
    docker build -t go-url-shortener .
    ```

2.  **Run the Container:**
    You need to pass the environment variables to the container using the `-e` flag. You also need to link it to a running Redis instance (either another container or a remote instance).

    ```bash
    docker run -d \
      -p 8443:8080 \
      -e HOST="http://localhost:8443" \
      -e REDIS="redis://your-redis-host:6379" \
      --name url-shortener-app \
      go-url-shortener
    ```
    *Note: Replace `your-redis-host` with the actual hostname or IP of your Redis server.*

---

## 🌐 API Endpoints

The API supports two main operations: creating a new short URL and redirecting from a short URL.

| Method | Endpoint | Description |
| :--- | :--- | :--- |
| **GET** | `/api?url=https://example.com&dur=360` | Creates a new short URL from a long URL. |
| **GET** | `/{shortCode}` | Redirects the user to the original long URL. |
| **POST** | `/api/bulk` | Creates multiple short URLS for a list of provided URLS. |
### 1. Shorten Multiple URLS (`POST /api/bulk`)

#### Request data example : 
```json
[
	{"url":"https://example1.com","duration":360},
	{"url":"https://example2.com","duration":360}, 
	....etc
]

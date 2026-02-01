# Requestarr 🎬

A fast, lightweight media request gateway for Sonarr & Radarr. Built in Go for maximum performance.

[![Docker Image](https://img.shields.io/badge/docker-ghcr.io%2Ficaruscore%2Frequestarr-blue)](https://github.com/IcarusCore/Requestarr/pkgs/container/requestarr)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

![Requestarr Screenshot](https://via.placeholder.com/800x400?text=Requestarr+UI)

## ✨ Features

- **🚀 Blazing Fast** - Built in Go with ~50ms startup time and ~15MB memory usage
- **🎨 Modern UI** - Beautiful, responsive interface for discovering and requesting media
- **📺 TV & Movies** - Full support for both Sonarr (TV) and Radarr (Movies)
- **🔍 Discovery** - Browse trending, top-rated, and new releases via TMDB
- **⭐ Ratings** - View Rotten Tomatoes, IMDB, and Metacritic scores
- **🔔 Notifications** - Discord and ntfy.sh support for request alerts
- **🛡️ Admin Panel** - Approve/reject requests, configure settings
- **🐳 Docker Ready** - Simple one-command deployment

## 🚀 Quick Start

### Docker Run (Recommended)

```bash
docker run -d \
  --name requestarr \
  -p 5000:5000 \
  -v $(pwd)/config:/config \
  -e ADMIN_PASSWORD=changeme \
  -e SECRET_KEY=$(openssl rand -hex 32) \
  -e TZ=America/New_York \
  --restart unless-stopped \
  ghcr.io/icaruscore/requestarr:latest
```

Then open `http://localhost:5000` in your browser.

### Docker Compose

```bash
# Create directory and download compose file
mkdir requestarr && cd requestarr
curl -O https://raw.githubusercontent.com/IcarusCore/Requestarr/main/docker-compose.yml

# Edit configuration
nano docker-compose.yml

# Start
docker compose up -d
```

## 📦 Installation Methods

### Method 1: Docker (Recommended)

#### Using `docker run`

```bash
docker run -d \
  --name requestarr \
  -p 5000:5000 \
  -v /path/to/config:/config \
  -e ADMIN_PASSWORD=your-secure-password \
  -e SECRET_KEY=your-random-secret-key \
  -e TZ=America/New_York \
  -e SONARR_URL=http://sonarr:8989 \
  -e SONARR_API_KEY=your-sonarr-api-key \
  -e RADARR_URL=http://radarr:7878 \
  -e RADARR_API_KEY=your-radarr-api-key \
  -e TMDB_API_KEY=your-tmdb-api-key \
  --restart unless-stopped \
  ghcr.io/icaruscore/requestarr:latest
```

#### Using Docker Compose

Create a `docker-compose.yml`:

```yaml
services:
  requestarr:
    image: ghcr.io/icaruscore/requestarr:latest
    container_name: requestarr
    ports:
      - "5000:5000"
    volumes:
      - ./config:/config
    environment:
      - ADMIN_PASSWORD=changeme
      - SECRET_KEY=change-me-please
      - TZ=America/New_York
      # Configure via environment or Web UI:
      # - SONARR_URL=http://sonarr:8989
      # - SONARR_API_KEY=your-api-key
      # - RADARR_URL=http://radarr:7878
      # - RADARR_API_KEY=your-api-key
      # - TMDB_API_KEY=your-tmdb-api-key
    restart: unless-stopped
```

Then run:

```bash
docker compose up -d
```

### Method 2: Build from Source

```bash
# Clone the repository
git clone https://github.com/IcarusCore/Requestarr.git
cd Requestarr

# Build (requires Go 1.21+ and GCC)
make build

# Run
./requestarr
```

### Method 3: Build Docker Image Locally

```bash
git clone https://github.com/IcarusCore/Requestarr.git
cd Requestarr

docker build -t requestarr .
docker run -d -p 5000:5000 -v $(pwd)/config:/config requestarr
```

## ⚙️ Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `5000` | Port to listen on |
| `DB_PATH` | `/config/requestarr.db` | SQLite database path |
| `ADMIN_PASSWORD` | `admin` | Admin panel password |
| `SECRET_KEY` | `change-me...` | Session encryption key (use random string!) |
| `TZ` | `UTC` | Timezone (e.g., `America/New_York`) |
| `SONARR_URL` | | Sonarr URL (e.g., `http://sonarr:8989`) |
| `SONARR_API_KEY` | | Sonarr API key |
| `RADARR_URL` | | Radarr URL (e.g., `http://radarr:7878`) |
| `RADARR_API_KEY` | | Radarr API key |
| `TMDB_API_KEY` | | TMDB API key (required for discovery) |
| `MDBLIST_API_KEY` | | MDBList API key (for Rotten Tomatoes ratings) |
| `DISCORD_WEBHOOK` | | Discord webhook URL for notifications |
| `NTFY_URL` | | ntfy server URL (e.g., `https://ntfy.sh`) |
| `NTFY_TOPIC` | | ntfy topic name |

### Web UI Configuration

All settings can be configured through the Admin panel:

1. Open `http://YOUR-SERVER:5000`
2. Click the **Admin** tab
3. Enter your admin password
4. Navigate to **Settings**
5. Configure your Sonarr, Radarr, TMDB, and notification settings

## 🔑 Getting API Keys

### TMDB (Required for Discovery)

1. Go to [themoviedb.org](https://www.themoviedb.org/)
2. Create a free account
3. Go to Settings → API → Create → Developer
4. Copy your **API Key (v3 auth)**

### MDBList (Optional - Rotten Tomatoes Ratings)

1. Go to [mdblist.com](https://mdblist.com/)
2. Create a free account
3. Go to [Preferences](https://mdblist.com/preferences/)
4. Copy your API Key

### Sonarr/Radarr

1. Open your Sonarr/Radarr web UI
2. Go to Settings → General → Security
3. Copy the **API Key**

## 🔧 Network Configuration

### Connecting to Sonarr/Radarr on the Same Docker Network

If Sonarr/Radarr are on a Docker network:

```yaml
services:
  requestarr:
    image: ghcr.io/icaruscore/requestarr:latest
    networks:
      - arr-network
    environment:
      - SONARR_URL=http://sonarr:8989
      - RADARR_URL=http://radarr:7878
      # ...

networks:
  arr-network:
    external: true
```

### Firewall Configuration

```bash
# Ubuntu/Debian with UFW
sudo ufw allow 5000/tcp

# CentOS/RHEL with firewalld
sudo firewall-cmd --permanent --add-port=5000/tcp
sudo firewall-cmd --reload
```

## 📊 Performance

| Metric | Requestarr (Go) | Typical Python Apps |
|--------|-----------------|---------------------|
| Startup time | ~50ms | ~2-3 seconds |
| Memory usage | ~15MB | ~50-100MB |
| Binary size | ~15MB | N/A (requires runtime) |
| Docker image | ~25MB | ~200-500MB |

## 📁 Project Structure

```
Requestarr/
├── cmd/
│   └── server/
│       ├── main.go              # Application entry point
│       └── frontend/
│           └── static/
│               └── index.html   # Web UI (embedded)
├── internal/
│   ├── cache/                   # In-memory caching
│   ├── handlers/                # HTTP request handlers
│   ├── models/                  # Database models
│   └── services/                # API integrations
│       ├── tmdb.go             # TMDB discovery
│       ├── sonarr.go           # Sonarr API
│       ├── radarr.go           # Radarr API
│       ├── ratings.go          # Ratings (RT/MDBList)
│       └── notifications.go    # Discord/ntfy
├── Dockerfile
├── docker-compose.yml
├── go.mod
└── README.md
```

## 🔍 Troubleshooting

### App won't start

```bash
# Check if port is in use
sudo lsof -i :5000

# Check Docker logs
docker logs requestarr
```

### Can't connect to Sonarr/Radarr

- Ensure URLs are accessible from the container
- Use container names (not `localhost`) when on the same Docker network
- Check API keys are correct
- Test connection in Admin → Settings

### Database errors

```bash
# Reset database (will lose all requests)
docker exec requestarr rm /config/requestarr.db
docker restart requestarr
```

## 🔄 Updating

```bash
# Docker Compose
docker compose pull
docker compose up -d

# Docker Run
docker pull ghcr.io/icaruscore/requestarr:latest
docker stop requestarr
docker rm requestarr
# Re-run your docker run command
```

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## 📄 License

MIT License - feel free to use, modify, and distribute.

---

Made with ❤️ for the self-hosted community

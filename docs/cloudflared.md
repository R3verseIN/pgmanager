# Cloudflare Tunnel Configuration Guide

This project comes with built-in support for Cloudflare Tunnels (`cloudflared`). It allows you to securely expose the Web UI and Database to the internet without opening any ports on your firewall or dealing with NAT routing.

By default, the Cloudflare Tunnel is **disabled** and consumes zero resources.

## 1. How to Enable the Tunnel

To enable the tunnel, you need to use the provided environment template:

1. Copy the `.env.example` file to `.env`:
   ```bash
   cp .env.example .env
   ```
2. Open the `.env` file and uncomment the two variables so it looks exactly like this:
   ```env
   COMPOSE_PROFILES=tunnel
   CLOUDFLARE_TUNNEL_TOKEN=your_token_here
   ```

*(Note: The `COMPOSE_PROFILES=tunnel` line is what tells Docker to magically boot the optional `cloudflared` container!)*

## 2. Generating Your Token

You need to get a Tunnel Token from Cloudflare:
1. Log into your **Cloudflare Zero Trust Dashboard**.
2. Go to **Networks** -> **Tunnels** and click **Create a tunnel**.
3. Choose **Cloudflared** and name your tunnel.
4. On the installation page, look at the "Install and run a connector" section. You will see a long string of text in the command starting with `ey...`. This is your token.
5. Copy that token and paste it into your `.env` file as `CLOUDFLARE_TUNNEL_TOKEN=ey...`

## 3. Configuring Service URLs in Cloudflare

When setting up your Public Hostnames in the Cloudflare Dashboard, you **must not** use `localhost` because Docker runs the tunnel in its own isolated network.

Instead, use the internal Docker service names for the Service URL:

### Web UI
To expose the Go Web Application, choose your Subdomain and Domain, and set the Service URL:
- **Service URL:** `http://app:8080`
*(Leave the Path field empty).*

### Database (PgBouncer)
To expose the Database for remote connections (requires Cloudflare Access/Spectrum for TCP routing):
- **Service URL:** `tcp://pgbouncer:6432`

## 4. Starting the Application

Once your `.env` file is saved and configured, simply run the normal Docker command:

```bash
docker compose up -d
```

Docker will automatically detect your profile, boot the `cloudflared` container, securely inject your token, and connect your server directly to Cloudflare's Edge!

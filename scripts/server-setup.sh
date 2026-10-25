#!/usr/bin/env bash
# Sets up a fresh Ubuntu server to run this fork for one person: whatnow behind Caddy for HTTPS,
# updates from the latest GitHub release every 10 minutes, and a nightly copy of the databases.
#
#   curl -fsSLO https://raw.githubusercontent.com/jonathanburman-create/whatnow/main/scripts/server-setup.sh
#   sudo bash server-setup.sh 35-190-172-58.sslip.io
#
# It asks for the Google sign-in client ID and secret the first time. Running it again is safe.
set -euo pipefail

REPO="jonathanburman-create/whatnow"
HOST="${1:?usage: sudo bash server-setup.sh <hostname>}"
HOST="${HOST,,}"

if [[ $EUID -ne 0 ]]; then
	echo "Run it with sudo: sudo bash server-setup.sh $HOST" >&2
	exit 1
fi

say() { printf '\n\033[1m==> %s\033[0m\n' "$*"; }

say "Installing Caddy and tools"
export DEBIAN_FRONTEND=noninteractive
apt-get update -q
apt-get install -yq caddy sqlite3 curl openssl

# The smallest free server has 1 GB of memory. A little swap keeps an occasional spike from
# killing the app.
if [[ ! -f /swapfile ]]; then
	say "Adding 1 GB of swap"
	fallocate -l 1G /swapfile
	chmod 600 /swapfile
	mkswap -q /swapfile
	swapon /swapfile
	echo '/swapfile none swap sw 0 0' >> /etc/fstab
fi

say "Creating the whatnow user and data folder"
id whatnow &>/dev/null || useradd --system --home /var/lib/whatnow --shell /usr/sbin/nologin whatnow
install -d -m 700 -o whatnow -g whatnow /var/lib/whatnow
install -d -m 700 -o root -g root /var/lib/whatnow-backups

if [[ ! -f /etc/whatnow.env ]]; then
	say "Google sign-in"
	echo "Paste the values from the OAuth client you made in Google Cloud."
	read -rp "Client ID: " client_id
	read -rsp "Client secret (hidden as you paste): " client_secret
	echo
	seal_key="$(openssl rand -base64 32)"
	install -m 600 -o root -g root /dev/null /etc/whatnow.env
	printf 'WHATNOW_GOOGLE_CLIENT_ID=%s\nWHATNOW_GOOGLE_CLIENT_SECRET=%s\nWHATNOW_SEAL_KEY=%s\n' \
		"$client_id" "$client_secret" "$seal_key" > /etc/whatnow.env
	echo
	echo "Your seal key encrypts any API keys you save in the app. Keep a copy in your password manager:"
	echo "  $seal_key"
fi

say "Installing the updater"
cat > /usr/local/bin/whatnow-update <<UPDATE
#!/usr/bin/env bash
# Installs the latest release of $REPO if it's newer than the one running.
set -euo pipefail
state=/var/lib/whatnow-update
mkdir -p "\$state"
latest="\$(curl -fsSI https://github.com/$REPO/releases/latest | tr -d '\r' | sed -n 's|^location: .*/tag/||Ip')"
if [[ -z "\$latest" ]]; then
	echo "no release found yet" >&2
	exit 0
fi
if [[ -x /usr/local/bin/whatnow && "\$(cat "\$state/tag" 2>/dev/null)" == "\$latest" ]]; then
	exit 0
fi
tmp="\$(mktemp)"
curl -fsSL -o "\$tmp" "https://github.com/$REPO/releases/download/\$latest/whatnow-linux-amd64"
install -m 755 "\$tmp" /usr/local/bin/whatnow
rm -f "\$tmp"
echo "\$latest" > "\$state/tag"
if systemctl is-enabled --quiet whatnow 2>/dev/null; then
	systemctl restart whatnow
fi
echo "installed \$latest"
UPDATE
chmod 755 /usr/local/bin/whatnow-update

cat > /etc/systemd/system/whatnow-update.service <<'UNIT'
[Unit]
Description=Install the latest whatnow release
After=network-online.target
Wants=network-online.target

[Service]
Type=oneshot
ExecStart=/usr/local/bin/whatnow-update
UNIT

cat > /etc/systemd/system/whatnow-update.timer <<'UNIT'
[Unit]
Description=Check for a new whatnow release every 10 minutes

[Timer]
OnBootSec=2min
OnUnitActiveSec=10min

[Install]
WantedBy=timers.target
UNIT

say "Downloading whatnow"
/usr/local/bin/whatnow-update

say "Installing the whatnow service"
cat > /etc/systemd/system/whatnow.service <<UNIT
[Unit]
Description=whatnow (hosted, for one person)
After=network-online.target
Wants=network-online.target

[Service]
User=whatnow
Group=whatnow
EnvironmentFile=/etc/whatnow.env
ExecStart=/usr/local/bin/whatnow host -addr 127.0.0.1:8080 -data /var/lib/whatnow -public-url https://$HOST -max-users 1
Restart=on-failure
RestartSec=5
NoNewPrivileges=true
ProtectSystem=strict
ReadWritePaths=/var/lib/whatnow
ProtectHome=true
PrivateTmp=true

[Install]
WantedBy=multi-user.target
UNIT

say "Installing the nightly backup"
cat > /usr/local/bin/whatnow-backup <<'BACKUP'
#!/usr/bin/env bash
# Copies every database safely, even while the app runs, and keeps 14 nights of copies.
set -euo pipefail
dest="/var/lib/whatnow-backups/$(date +%F)"
mkdir -p "$dest"
cd /var/lib/whatnow
find . -name '*.db' -print0 | while IFS= read -r -d '' db; do
	mkdir -p "$dest/$(dirname "$db")"
	sqlite3 "$db" ".backup '$dest/$db'"
done
find /var/lib/whatnow-backups -mindepth 1 -maxdepth 1 -type d -mtime +14 -exec rm -rf {} +
BACKUP
chmod 755 /usr/local/bin/whatnow-backup

cat > /etc/systemd/system/whatnow-backup.service <<'UNIT'
[Unit]
Description=Back up the whatnow databases

[Service]
Type=oneshot
ExecStart=/usr/local/bin/whatnow-backup
UNIT

cat > /etc/systemd/system/whatnow-backup.timer <<'UNIT'
[Unit]
Description=Back up the whatnow databases nightly

[Timer]
OnCalendar=*-*-* 04:00:00
Persistent=true

[Install]
WantedBy=timers.target
UNIT

say "Setting up HTTPS for $HOST"
cat > /etc/caddy/Caddyfile <<CADDY
$HOST {
	reverse_proxy 127.0.0.1:8080
}
CADDY

systemctl daemon-reload
systemctl enable --now whatnow whatnow-update.timer whatnow-backup.timer
systemctl reload caddy || systemctl restart caddy

say "Checking it answers"
for _ in $(seq 1 30); do
	if curl -fsS -o /dev/null "https://$HOST/api/v1/session"; then
		echo "All set. Open https://$HOST on your phone and Mac, and sign in with Google."
		exit 0
	fi
	sleep 2
done
echo "The app is installed, but https://$HOST isn't answering yet." >&2
echo "Check: systemctl status whatnow caddy, and journalctl -u whatnow -u caddy -n 50" >&2
exit 1

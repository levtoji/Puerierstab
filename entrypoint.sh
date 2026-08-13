#!/bin/sh
set -e

# A mounted volume (e.g. Railway's /data) overrides whatever ownership was
# baked into the image at build time. Ensure the volume is writable by the
# unprivileged runtime user, then drop privileges and start the bot.
chown -R appuser:appuser /data

exec su-exec appuser /usr/local/bin/puerierstab

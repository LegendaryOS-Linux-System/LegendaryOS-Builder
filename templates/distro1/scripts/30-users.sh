#!/bin/sh
# Konto live + grupy. /home jest zapisywalny; hasło live ustaw w instalatorze, nie tutaj.
set -eu
getent group wheel >/dev/null || groupadd -r wheel
id live >/dev/null 2>&1 || useradd -m -G wheel -s /bin/bash live
passwd -d live || true

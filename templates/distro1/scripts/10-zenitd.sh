#!/bin/sh
# Włącza podstawowe jednostki zenitd offline (chroot obrazu). Wymaga zenitctl z kroku "zenitd".
set -eu
zenitctl enable udevd.service || true
zenitctl enable getty@tty1.service || true
zenitctl enable legendary-state.service
zenitctl enable legendary-tools.service
zenitctl enable NetworkManager.service || true
ln -sf /usr/lib/zenit/system/graphical.target /etc/zenit/system/default.target

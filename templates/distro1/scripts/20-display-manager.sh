#!/bin/sh
# SDDM jako display manager (docelowo HDM). Sesja: Blue Environment.
set -eu
zenitctl enable sddm.service || true

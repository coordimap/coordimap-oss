#!/bin/sh
systemctl daemon-reload || true
systemctl enable coordimap-agent || true
systemctl restart coordimap-agent || true

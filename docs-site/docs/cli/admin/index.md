---
title: mr admin
description: Server administration commands
sidebar_label: admin
---

# mr admin

The `mr admin` command group covers server administration — runtime stats and
runtime configuration management.

## Subcommands

- [`stats`](./stats.md) — Show server and data statistics (the default when
  no subcommand is given).
- [`settings`](./settings.md) — View and manage runtime configuration overrides.

## Examples

    mr admin                  # shorthand for `mr admin stats`
    mr admin stats --server-only  # server stats only
    mr admin settings list

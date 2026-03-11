# vibe-c2-telegram-channel

First Telegram channel module for Vibe C2 (`step 1` minimal text-based transport).

## Install

```bash
go mod tidy
```

## Config

Edit `configs/channel.example.yaml`:

- `bot_token`
- `c2_sync_base_url`
- `profiles_file`

## Message format (v0.1.0)

Inbound from implant/session to bot:

```text
p:<profile-id>
id:<id>
<encrypted_data>
```

or without profile hint:

```text
id:<id>
<encrypted_data>
```

Outbound back to chat:

```text
id:<outbound-id>
<outbound-encrypted_data>
```

## Run

```bash
go run ./cmd/telegram-channel
```

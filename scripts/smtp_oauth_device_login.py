#!/usr/bin/env python3
"""Microsoft 365 SMTP XOAUTH2 device-code login helper.

Runs the OAuth2 device authorization flow against Microsoft identity platform,
prompts you to sign in via a browser, and prints the refresh_token to paste into
new-api's SMTP OAuth settings (option key: SMTPOAuthRefreshToken).

Usage:
  python3 scripts/smtp_oauth_device_login.py \
      --client-id <APP_CLIENT_ID> \
      --tenant <TENANT_ID_or_common>

No third-party dependencies; uses only the Python standard library.

The app registration must have:
  - Allow public client flows = Yes
  - Delegated permissions: offline_access, https://outlook.office365.com/SMTP.Send
"""
import argparse
import json
import sys
import time
import urllib.error
import urllib.parse
import urllib.request

SCOPE = "https://outlook.office365.com/SMTP.Send offline_access"


def _post(url: str, data: dict) -> dict:
    body = urllib.parse.urlencode(data).encode()
    req = urllib.request.Request(
        url, data=body, headers={"Content-Type": "application/x-www-form-urlencoded"}
    )
    try:
        with urllib.request.urlopen(req, timeout=30) as resp:
            return json.load(resp)
    except urllib.error.HTTPError as exc:
        try:
            return json.load(exc)
        except Exception:
            return {"error": "http_error", "error_description": f"HTTP {exc.code}"}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--client-id", required=True, help="Entra application (client) ID")
    parser.add_argument(
        "--tenant",
        default="common",
        help='Directory (tenant) ID, or "common" for multi-tenant (default: common)',
    )
    args = parser.parse_args()

    base = f"https://login.microsoftonline.com/{args.tenant}/oauth2/v2.0"
    device_url = f"{base}/devicecode"
    token_url = f"{base}/token"

    dc = _post(device_url, {"client_id": args.client_id, "scope": SCOPE})
    if "device_code" not in dc:
        print("device code request failed:", json.dumps(dc, indent=2), file=sys.stderr)
        return 1

    print("\n==== Microsoft 365 SMTP OAuth device login ====")
    print(dc.get("message", ""))
    print("\nVerification URL:", dc.get("verification_uri"))
    print("User code:", dc.get("user_code"))
    print("\nOpen the URL in a browser, sign in with the SENDER mailbox account,")
    print("and grant the requested SMTP.Send + offline_access permissions.\n")

    interval = int(dc.get("interval", 5))
    expires_in = int(dc.get("expires_in", 900))
    device_code = dc["device_code"]
    deadline = time.time() + expires_in

    while time.time() < deadline:
        time.sleep(interval)
        tok = _post(
            token_url,
            {
                "grant_type": "urn:ietf:params:oauth:grant-type:device_code",
                "client_id": args.client_id,
                "device_code": device_code,
            },
        )
        err = tok.get("error")
        if err == "authorization_pending":
            continue
        if err == "slow_down":
            interval += 5
            continue
        if err:
            print("token error:", json.dumps(tok, indent=2), file=sys.stderr)
            return 1
        refresh_token = tok.get("refresh_token", "")
        access_token = tok.get("access_token", "")
        if not refresh_token:
            print("no refresh_token returned; ensure offline_access scope is granted", file=sys.stderr)
            print(json.dumps(tok, indent=2), file=sys.stderr)
            return 1
        print("\n==== SUCCESS ====")
        print("access_token acquired (expires_in=%s s)" % tok.get("expires_in"))
        print("\n--- Paste this into new-api SMTP OAuth settings ---")
        print("SMTPOAuthClientId   =", args.client_id)
        print("SMTPOAuthTenantId   =", args.tenant)
        print("SMTPOAuthRefreshToken =")
        print(refresh_token)
        print("--- end ---\n")
        # Also emit machine-readable line for scripting.
        print("REFRESH_TOKEN_LINE:" + refresh_token)
        return 0

    print("device code expired before authorization completed", file=sys.stderr)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())

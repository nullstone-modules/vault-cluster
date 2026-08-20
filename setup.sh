#!/usr/bin/env bash
# One-command local developer environment. Delegates to local/setup.sh.
set -euo pipefail
exec "$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)/local/setup.sh" "$@"

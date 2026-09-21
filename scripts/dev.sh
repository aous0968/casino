#!/usr/bin/env bash
set -uo pipefail

pass=0
fail=0

check() {
  local name="$1"; shift
  if "$@" >/dev/null 2>&1; then
    echo "  ✅ $name"
    pass=$((pass+1))
  else
    echo "  ❌ $name"
    fail=$((fail+1))
  fi
}

echo "Checking local infrastructure..."
check "postgres reachable"        pg_isready -h localhost -p 5432
check "postgres casino login"     env PGPASSWORD=casino psql -h localhost -U casino -d casino -c "SELECT 1" 
check "redis reachable"           redis-cli ping
check "rabbitmq running"          sudo rabbitmqctl status
check "rabbitmq management UI"    curl -sf -u casino:casino http://localhost:15672/api/overview

echo
echo "Passed: $pass   Failed: $fail"
exit $(( fail > 0 ? 1 : 0 ))

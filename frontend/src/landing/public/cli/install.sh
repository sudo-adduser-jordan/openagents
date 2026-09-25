#!/bin/sh
# Open Agents CLI installer
#
# Usage:
#   curl -fsSL https://orchestrator.inc/cli/install.sh | sh
#
# Installs the `open-agents` CLI via the Homebrew tap. For the desktop app, download it
# from https://github.com/sudo-adduser-jordan/open-agents/releases/latest.

set -eu

BOLD='\033[1m'
GREEN='\033[32m'
RED='\033[31m'
RESET='\033[0m'

info() { printf "${GREEN}==>${RESET} %s\n" "$1" >&2; }
error() { printf "${RED}error:${RESET} %s\n" "$1" >&2; exit 1; }

RELEASES_URL="https://github.com/sudo-adduser-jordan/open-agents/releases/latest"

main() {
    printf "${BOLD}Installing Open Agents${RESET}\n\n"

    if ! command -v brew >/dev/null 2>&1; then
        error "Homebrew is required. Install it from https://brew.sh, or download the desktop app from ${RELEASES_URL}"
    fi

    info "Installing via Homebrew"
    brew install sudo-adduser-jordan/tap/open-agents

    printf "\n${GREEN}${BOLD}Installed!${RESET}\n"
    printf "Run ${BOLD}open-agents${RESET} to get started.\n"
}

main "$@"

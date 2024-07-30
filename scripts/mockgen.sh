#!/bin/bash

FILENAME=${1%.*}
DEBUG=${DEBUG:-false}
SUB_PACKAGE=mocks/
MOCK_PACKAGE=""

function echoDebug() {
    if [[ "${DEBUG}" == "true" ]]; then
        echo "[mockgen.sh] [DEBUG] $@"
    fi
}

while [ -n "$1" ]; do
    case $1 in
    "--no-sub-package")
        SUB_PACKAGE=""
    ;;
    "--package")
        shift
        if [ -n "$1" ]; then
            MOCK_PACKAGE="-package=$1"
        fi
    ;;
    esac
    shift
done

test -z "${SUB_PACKAGE}" || mkdir -p "${SUB_PACKAGE}"

PROJECT_DIR="${PROJECT_DIR:-}"
echoDebug "Generating mocks for file: ${PWD#${PROJECT_DIR}/}/$FILE"
mockgen -source="${FILENAME}.go" -destination="${SUB_PACKAGE}${FILENAME}_mock.go" "${MOCK_PACKAGE}"

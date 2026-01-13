#!/bin/bash

# Maestro E2E Test Commands for maestro-runner
# Usage: ./scripts/run-maestro-e2e.sh [test-name]
#
# Available tests:
#   assertVisible    - Single assertion test
#   repeat           - Repeat loop test
#   inputText        - Text input test
#   back             - Back button test
#   runFlow          - RunFlow with inline JS test
#   commands         - All command tests (requires inputRandom* support)
#   tour             - Full commands tour (requires inputRandom* support)
#   wikipedia        - Wikipedia app test
#   orientation      - Set orientation test (requires extended orientations)
#   all              - Run all supported tests

set -e

RUNNER_DIR="/Users/omnarayan/work/go/src/maestro-runner"
MAESTRO_DIR="/Users/omnarayan/work/support-tools/Maestro"
E2E_WORKSPACES="$MAESTRO_DIR/e2e/workspaces"
E2E_APPS="$MAESTRO_DIR/e2e/apps"

# Build runner if not exists
if [ ! -f "$RUNNER_DIR/maestro-runner" ]; then
    echo "Building maestro-runner..."
    cd "$RUNNER_DIR" && go build .
fi

RUNNER="$RUNNER_DIR/maestro-runner"

run_test() {
    echo "========================================"
    echo "Running: $1"
    echo "========================================"
    $RUNNER --platform android --app-file "$3" test "$2" ${@:4}
}

case "${1:-assertVisible}" in
    assertVisible)
        run_test "Assert Visible" \
            "$E2E_WORKSPACES/demo_app/commands/assertVisible.yaml" \
            "$E2E_APPS/demo_app.apk"
        ;;

    repeat)
        run_test "Repeat" \
            "$E2E_WORKSPACES/demo_app/commands/repeat.yaml" \
            "$E2E_APPS/demo_app.apk"
        ;;

    inputText)
        run_test "Input Text" \
            "$E2E_WORKSPACES/demo_app/commands/inputText.yaml" \
            "$E2E_APPS/demo_app.apk"
        ;;

    back)
        run_test "Back" \
            "$E2E_WORKSPACES/demo_app/commands/back.yaml" \
            "$E2E_APPS/demo_app.apk"
        ;;

    runFlow)
        run_test "Run Flow" \
            "$E2E_WORKSPACES/demo_app/commands/runFlow.yaml" \
            "$E2E_APPS/demo_app.apk"
        ;;

    commands)
        run_test "All Commands" \
            "$E2E_WORKSPACES/demo_app/commands/" \
            "$E2E_APPS/demo_app.apk"
        ;;

    tour)
        run_test "Commands Tour" \
            "$E2E_WORKSPACES/demo_app/commands_tour.yaml" \
            "$E2E_APPS/demo_app.apk"
        ;;

    wikipedia)
        run_test "Wikipedia" \
            "$E2E_WORKSPACES/wikipedia/android-flow.yaml" \
            "$E2E_APPS/wikipedia.apk"
        ;;

    orientation)
        run_test "Set Orientation" \
            "$E2E_WORKSPACES/setOrientation/test-set-orientation-flow.yaml" \
            "$E2E_APPS/setOrientation.apk"
        ;;

    all)
        $0 assertVisible
        $0 repeat
        $0 inputText
        $0 back
        $0 runFlow
        $0 wikipedia
        ;;

    *)
        echo "Unknown test: $1"
        echo "Available: assertVisible, commands, tour, wikipedia, orientation, all"
        exit 1
        ;;
esac

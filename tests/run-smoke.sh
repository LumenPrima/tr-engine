#!/bin/bash
cd "$(dirname "$0")/.."
npx playwright test --config=tests/playwright.config.js

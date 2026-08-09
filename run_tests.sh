#!/usr/bin/env sh
set -eu
python3 -m unittest discover -s ml/unit_policy/tests -v

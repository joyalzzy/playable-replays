"""Train a small temporary model and print representative decisions."""
from __future__ import annotations

import argparse
import json
from pathlib import Path

from .carry_safety import demo, train_carry_model


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--seed", type=int, default=91)
    parser.add_argument("--bootstrap-examples", type=int, default=800)
    parser.add_argument("--curriculum-examples", type=int, default=1000)
    parser.add_argument("--epochs", type=int, default=10)
    parser.add_argument("--save-model", type=Path)
    args = parser.parse_args()
    model, _ = train_carry_model(seed=args.seed, bootstrap_examples=args.bootstrap_examples, curriculum_count=args.curriculum_examples, epochs=args.epochs, output=args.save_model, policy_version="replay-guided-demo-v1")
    print(json.dumps(demo(model), indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

import json
import tempfile
import unittest
from dataclasses import replace
from pathlib import Path

from ml.evaluate.detector import (
    evaluate_pack,
    load_pack,
    render_markdown,
    report_as_dict,
)


ROOT = Path(__file__).resolve().parents[2]
PACK_PATH = ROOT / "fixtures" / "telemetry-evaluation" / "manifest.json"


class DetectorEvaluationTests(unittest.TestCase):
    def test_manifest_rejects_unknown_fields_before_evaluation(self):
        payload = json.loads(PACK_PATH.read_text(encoding="utf-8"))
        payload["unexpected"] = True
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "manifest.json"
            path.write_text(json.dumps(payload), encoding="utf-8")

            with self.assertRaisesRegex(ValueError, "unknown fields"):
                load_pack(path)

    def test_checked_in_pack_covers_categories_and_passes_regression_gate(self):
        report = evaluate_pack(load_pack(PACK_PATH))

        self.assertTrue(report.passed, report.failures)
        self.assertEqual(report.metrics.case_count, 7)
        self.assertEqual(report.metrics.expected_highlight_count, 7)
        self.assertEqual(report.metrics.true_positive_count, 7)
        self.assertEqual(report.metrics.false_positive_count, 0)
        self.assertEqual(report.metrics.missed_highlight_count, 0)
        self.assertEqual(report.metrics.precision, 1.0)
        self.assertEqual(report.metrics.recall, 1.0)
        self.assertEqual(report.metrics.category_accuracy, 1.0)
        self.assertEqual(report.metrics.top_rank_accuracy, 1.0)
        self.assertEqual(report.metrics.false_positive_match_rate, 0.0)

    def test_wrong_category_fails_category_regression_threshold(self):
        pack = load_pack(PACK_PATH)
        first_case = pack.cases[0]
        wrong_expected = replace(
            first_case.expected_highlights[0], category="escape"
        )
        changed_case = replace(
            first_case,
            expected_highlights=(wrong_expected, *first_case.expected_highlights[1:]),
        )
        changed_pack = replace(pack, cases=(changed_case, *pack.cases[1:]))

        report = evaluate_pack(changed_pack)

        self.assertFalse(report.passed)
        self.assertAlmostEqual(report.metrics.category_accuracy, 6 / 7)
        self.assertTrue(
            any("category accuracy" in failure for failure in report.failures)
        )

    def test_highlight_in_ordinary_match_counts_as_false_positive(self):
        pack = load_pack(PACK_PATH)
        ordinary_case = pack.cases[-1]
        false_positive_case = replace(
            ordinary_case,
            telemetry_path=pack.cases[1].telemetry_path,
        )
        changed_pack = replace(pack, cases=(*pack.cases[:-1], false_positive_case))

        report = evaluate_pack(changed_pack)

        self.assertFalse(report.passed)
        self.assertEqual(report.metrics.false_positive_count, 1)
        self.assertEqual(report.metrics.false_positive_match_rate, 1.0)
        self.assertTrue(
            any("false-positive match rate" in failure for failure in report.failures)
        )

    def test_reports_have_stable_human_and_machine_readable_metrics(self):
        report = evaluate_pack(load_pack(PACK_PATH))

        markdown = render_markdown(report)
        machine = report_as_dict(report)

        self.assertIn("Precision: 100.0%", markdown)
        self.assertIn("ordinary-laning", markdown)
        self.assertIn("PASS", markdown)
        self.assertTrue(machine["passed"])
        self.assertEqual(machine["metrics"]["categoryAccuracy"], 1.0)
        self.assertEqual(len(machine["cases"]), 7)


if __name__ == "__main__":
    unittest.main()

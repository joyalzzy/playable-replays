import json
import tempfile
import unittest
from pathlib import Path

from ml.highlight import Candidate, Signals, score
from ml.telemetry import (
    SemanticEvidence,
    TelemetryFrame,
    TelemetryUnit,
    detection_record,
    extract_signals,
    load_frames,
    one_versus_many_unit_ids,
    select_pivotal_windows,
    semantic_evidence,
    signals_for_candidate,
    successful_escape_unit_ids,
    team_fight_reversal_second,
    telemetry_windows,
    validate_frames,
)


def unit(
    unit_id: str,
    team: str,
    *,
    x: float,
    y: float,
    hp: float = 100,
    gold: float = 1000,
) -> TelemetryUnit:
    return TelemetryUnit(unit_id, team, x, y, hp, 100, gold, hp > 0)


def frame(
    second: int,
    probability: float,
    *,
    blue_x: float = 50,
    red_x: float = 50,
    blue_hp: float = 100,
    red_hp: float = 100,
    blue_gold: float = 1000,
    red_gold: float = 1000,
    events: tuple[str, ...] = (),
) -> TelemetryFrame:
    return TelemetryFrame(
        second,
        probability,
        (
            unit(
                "blue-carry",
                "blue",
                x=blue_x,
                y=50,
                hp=blue_hp,
                gold=blue_gold,
            ),
            unit(
                "red-carry",
                "red",
                x=red_x,
                y=50,
                hp=red_hp,
                gold=red_gold,
            ),
        ),
        events,
    )


class TelemetryTests(unittest.TestCase):
    def test_extracts_all_four_normalized_signals(self):
        frames = (
            frame(0, 0.4, blue_x=0, red_x=100),
            frame(
                10,
                0.8,
                blue_hp=50,
                red_hp=100,
                blue_gold=500,
                red_gold=1500,
                events=("damage",) * 10,
            ),
        )

        signals = extract_signals(frames)

        self.assertAlmostEqual(signals.win_probability_swing, 0.4)
        self.assertAlmostEqual(signals.event_density, 0.5)
        self.assertAlmostEqual(signals.entity_proximity, 0.0)
        self.assertAlmostEqual(signals.resource_asymmetry, 0.5)

    def test_rejects_out_of_order_frames_and_roster_drift(self):
        with self.assertRaisesRegex(ValueError, "strictly increasing"):
            validate_frames((frame(1, 0.5), frame(1, 0.6)))

        changed_team = TelemetryFrame(
            2,
            0.6,
            (
                unit("blue-carry", "red", x=50, y=50),
                unit("red-carry", "red", x=50, y=50),
            ),
        )
        with self.assertRaisesRegex(ValueError, "stable across frames"):
            validate_frames((frame(1, 0.5), changed_team))

    def test_irregular_windows_are_fully_covered(self):
        frames = tuple(
            frame(second, 0.5)
            for second in (0, 3, 7, 13, 15, 21)
        )

        windows = telemetry_windows(frames, window_seconds=12, stride_seconds=4)

        self.assertEqual([(start, end) for start, end, _ in windows], [(0, 13), (7, 21)])

    def test_overlapping_pivotal_windows_are_suppressed(self):
        frames = tuple(
            frame(
                second,
                0.9 if second == 12 else 0.1,
                events=("damage",),
            )
            for second in range(25)
        )

        raw = telemetry_windows(frames, window_seconds=12, stride_seconds=2)
        candidates = select_pivotal_windows(
            frames,
            threshold=0.6,
            window_seconds=12,
            stride_seconds=2,
            max_overlap_fraction=0.49,
        )

        self.assertGreater(len(raw), len(candidates))
        self.assertEqual(
            [(candidate.start_second, candidate.end_second) for candidate in candidates],
            [(0, 12), (8, 20)],
        )
        self.assertIn("win-probability-swing", candidates[0].reason_tags)

    def test_one_versus_many_requires_sustained_isolation(self):
        def engagement_frame(second: int, *, ally_x: float = 90) -> TelemetryFrame:
            return TelemetryFrame(
                second,
                0.5,
                (
                    unit("blue-carry", "blue", x=50, y=50),
                    unit("blue-support", "blue", x=ally_x, y=50),
                    unit("red-top", "red", x=44, y=50),
                    unit("red-jungle", "red", x=56, y=50),
                ),
            )

        sustained = tuple(engagement_frame(second) for second in (0, 1, 2))
        interrupted = (
            engagement_frame(0),
            engagement_frame(1, ally_x=52),
            engagement_frame(2),
        )

        self.assertEqual(one_versus_many_unit_ids(sustained), ("blue-carry",))
        self.assertEqual(one_versus_many_unit_ids(interrupted), ())

    def test_selected_window_gets_one_versus_many_reason_tag(self):
        frames = tuple(
            TelemetryFrame(
                second,
                0.9 if second >= 2 else 0.1,
                (
                    unit("blue-carry", "blue", x=50, y=50),
                    unit("blue-support", "blue", x=90, y=50),
                    unit("red-top", "red", x=44, y=50),
                    unit("red-jungle", "red", x=56, y=50),
                ),
                ("damage",),
            )
            for second in range(13)
        )

        candidates = select_pivotal_windows(frames, window_seconds=12)

        self.assertEqual(len(candidates), 1)
        self.assertIn("one-versus-many", candidates[0].reason_tags)

    def test_one_versus_many_parameters_are_bounded(self):
        frames = (frame(0, 0.5), frame(2, 0.5))
        with self.assertRaisesRegex(ValueError, "engagement_radius"):
            one_versus_many_unit_ids(frames, engagement_radius=0)
        with self.assertRaisesRegex(ValueError, "minimum_exposure_seconds"):
            one_versus_many_unit_ids(frames, minimum_exposure_seconds=True)

    def test_successful_escape_requires_sustained_safe_separation(self):
        def escape_frame(
            second: int, opponent_x: float, *, opponent_hp: float = 100
        ) -> TelemetryFrame:
            return TelemetryFrame(
                second,
                0.5,
                (
                    unit("blue-carry", "blue", x=50, y=50, hp=25),
                    unit("red-jungle", "red", x=opponent_x, y=50, hp=opponent_hp),
                ),
            )

        escaped = (
            escape_frame(0, 55),
            escape_frame(2, 90),
            escape_frame(4, 90),
        )
        pressure_returns = (
            escape_frame(0, 55),
            escape_frame(2, 90),
            escape_frame(4, 55),
        )
        opponent_eliminated = (
            escape_frame(0, 55),
            escape_frame(2, 90, opponent_hp=0),
            escape_frame(4, 90, opponent_hp=0),
        )

        self.assertEqual(successful_escape_unit_ids(escaped), ("blue-carry",))
        self.assertEqual(successful_escape_unit_ids(pressure_returns), ())
        self.assertEqual(successful_escape_unit_ids(opponent_eliminated), ())

    def test_selected_window_gets_successful_escape_reason_tag(self):
        frames = tuple(
            TelemetryFrame(
                second,
                0.1 if second == 0 else 0.9,
                (
                    unit("blue-carry", "blue", x=50, y=50, hp=25),
                    unit(
                        "red-jungle",
                        "red",
                        x=55 if second == 0 else 90,
                        y=50,
                    ),
                ),
                ("damage",),
            )
            for second in range(13)
        )

        candidates = select_pivotal_windows(frames, window_seconds=12)

        self.assertEqual(len(candidates), 1)
        self.assertIn("successful-escape", candidates[0].reason_tags)

    def test_successful_escape_parameters_are_bounded(self):
        frames = (frame(0, 0.5), frame(2, 0.5))
        with self.assertRaisesRegex(ValueError, "safe_radius"):
            successful_escape_unit_ids(frames, safe_radius=10)
        with self.assertRaisesRegex(ValueError, "low_health_fraction"):
            successful_escape_unit_ids(frames, low_health_fraction=1.1)
        with self.assertRaisesRegex(ValueError, "minimum_safe_seconds"):
            successful_escape_unit_ids(frames, minimum_safe_seconds=True)

    def test_team_fight_reversal_requires_turning_point_and_combat(self):
        reversal = (
            frame(0, 0.7, events=("damage",)),
            frame(1, 0.3, events=("kill",)),
            frame(2, 0.8),
        )
        monotonic = (
            frame(0, 0.2, events=("damage",)),
            frame(1, 0.5, events=("kill",)),
            frame(2, 0.8),
        )
        no_combat = (
            frame(0, 0.7),
            frame(1, 0.3),
            frame(2, 0.8),
        )
        distant = (
            frame(0, 0.7, blue_x=0, red_x=100, events=("damage",)),
            frame(1, 0.3, blue_x=0, red_x=100, events=("kill",)),
            frame(2, 0.8, blue_x=0, red_x=100),
        )

        self.assertEqual(team_fight_reversal_second(reversal), 1)
        self.assertIsNone(team_fight_reversal_second(monotonic))
        self.assertIsNone(team_fight_reversal_second(no_combat))
        self.assertIsNone(team_fight_reversal_second(distant))

    def test_selected_window_gets_team_fight_reversal_reason_tag(self):
        frames = tuple(
            frame(
                second,
                0.2 if second == 6 else 0.85 if second == 12 else 0.75,
                events=("damage", "kill"),
            )
            for second in range(13)
        )

        candidates = select_pivotal_windows(frames, window_seconds=12)

        self.assertEqual(len(candidates), 1)
        self.assertIn("team-fight-reversal", candidates[0].reason_tags)

    def test_team_fight_reversal_parameters_are_bounded(self):
        frames = (
            frame(0, 0.7, events=("damage",)),
            frame(1, 0.3, events=("kill",)),
            frame(2, 0.8),
        )
        with self.assertRaisesRegex(ValueError, "minimum_reversal_swing"):
            team_fight_reversal_second(frames, minimum_reversal_swing=0)
        with self.assertRaisesRegex(ValueError, "engagement_radius"):
            team_fight_reversal_second(frames, engagement_radius=0)
        with self.assertRaisesRegex(ValueError, "minimum_combat_events"):
            team_fight_reversal_second(frames, minimum_combat_events=True)

    def test_semantic_evidence_matches_selected_window_tags(self):
        frames = tuple(
            TelemetryFrame(
                second,
                0.2 if second == 6 else 0.85 if second == 12 else 0.75,
                (
                    unit("blue-carry", "blue", x=50, y=50),
                    unit("blue-support", "blue", x=90, y=50),
                    unit("red-top", "red", x=44, y=50),
                    unit("red-jungle", "red", x=56, y=50),
                ),
                ("damage", "kill"),
            )
            for second in range(13)
        )

        candidate = select_pivotal_windows(frames, window_seconds=12)[0]
        evidence = semantic_evidence(frames, candidate)

        self.assertEqual(evidence.one_versus_many_unit_ids, ("blue-carry",))
        self.assertEqual(evidence.successful_escape_unit_ids, ())
        self.assertEqual(evidence.team_fight_reversal_second, 6)
        self.assertIn("one-versus-many", candidate.reason_tags)
        self.assertIn("team-fight-reversal", candidate.reason_tags)

    def test_detection_record_is_versioned_and_json_serializable(self):
        signals = Signals(0.8, 0.7, 0.2, 0.6)
        candidate = Candidate(10, 22, score(signals), ("team-fight-reversal",))
        evidence = SemanticEvidence(("blue-carry",), (), 16)

        record = detection_record(candidate, evidence, signals)

        self.assertEqual(record["schemaVersion"], "1.0")
        self.assertEqual(record["score"], 0.75)
        self.assertEqual(
            record["signals"],
            {
                "winProbabilitySwing": 0.8,
                "eventDensity": 0.7,
                "entityProximity": 0.2,
                "resourceAsymmetry": 0.6,
            },
        )
        self.assertEqual(
            record["semanticEvidence"],
            {
                "oneVersusManyUnitIds": ["blue-carry"],
                "successfulEscapeUnitIds": [],
                "teamFightReversalSecond": 16,
            },
        )
        json.dumps(record)

    def test_candidate_signals_recompute_score_and_reject_mismatch(self):
        frames = tuple(
            frame(
                second,
                0.85 if second == 12 else 0.1,
                events=("damage",),
            )
            for second in range(13)
        )
        candidate = select_pivotal_windows(
            frames, threshold=0.6, window_seconds=12
        )[0]
        signals = signals_for_candidate(frames, candidate)

        self.assertAlmostEqual(score(signals), candidate.score)
        detection_record(candidate, semantic_evidence(frames, candidate), signals)
        with self.assertRaisesRegex(ValueError, "does not match"):
            detection_record(candidate, SemanticEvidence(), Signals(0, 0, 1, 0))

    def test_semantic_evidence_rejects_uncovered_candidate_bounds(self):
        frames = (frame(0, 0.5), frame(12, 0.5))

        with self.assertRaisesRegex(ValueError, "fully covered"):
            semantic_evidence(frames, Candidate(1, 12, 0.7, ()))

    def test_load_frames_enforces_version_and_unknown_fields(self):
        payload = {
            "version": "1.0",
            "frames": [
                {
                    "second": second,
                    "winProbability": 0.5,
                    "events": ["damage"],
                    "units": [
                        {
                            "id": "blue-carry",
                            "team": "blue",
                            "position": {"x": 40, "y": 50},
                            "hp": 100,
                            "maxHp": 100,
                            "gold": 1000,
                        },
                        {
                            "id": "red-carry",
                            "team": "red",
                            "position": {"x": 60, "y": 50},
                            "hp": 100,
                            "maxHp": 100,
                            "gold": 1000,
                        },
                    ],
                }
                for second in (0, 12)
            ],
        }
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "telemetry.json"
            path.write_text(json.dumps(payload), encoding="utf-8")
            loaded = load_frames(path)
            self.assertEqual([item.second for item in loaded], [0, 12])

            payload["frames"][0]["unexpected"] = True
            path.write_text(json.dumps(payload), encoding="utf-8")
            with self.assertRaisesRegex(ValueError, "unknown fields"):
                load_frames(path)

    def test_rejects_unknown_event_types(self):
        with self.assertRaisesRegex(ValueError, "unknown event types"):
            validate_frames(
                (
                    frame(0, 0.5, events=("chat",)),
                    frame(1, 0.5),
                )
            )

    def test_selection_parameters_are_bounded(self):
        frames = (frame(0, 0.5), frame(12, 0.5))
        with self.assertRaisesRegex(ValueError, "threshold"):
            select_pivotal_windows(frames, threshold=1.1)
        with self.assertRaisesRegex(ValueError, "max_overlap_fraction"):
            select_pivotal_windows(frames, max_overlap_fraction=-0.1)


if __name__ == "__main__":
    unittest.main()

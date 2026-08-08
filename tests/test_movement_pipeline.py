import json
import unittest

from ml.movement_pipeline import (
    DRAGON_OBJECTIVE_TYPES,
    balance_training_records,
    build_training_example,
    grouped_split,
    positive_fraction,
    record_order_key,
    select_eligible_records,
    score_movement_prediction,
    summarize_heldout_results,
    validate_record,
)


def movement_record(
    match_id="match-1",
    *,
    movement_index=0,
    won=True,
    secured=True,
    objective_type="baron_nashor",
    critical=True,
    committed=True,
    seconds=20,
    fight_active=True,
):
    return {
        "matchId": match_id,
        "tick": 100,
        "movementIndex": movement_index,
        "state": {
            "controlledPlayer": {
                "id": "controlled",
                "alive": True,
                "position": {"x": 50, "z": 50},
            },
            "teammates": [
                {"id": "top", "alive": True, "position": {"x": 48, "z": 51}},
                {"id": "jungle", "alive": True, "position": {"x": 52, "z": 49}},
                {"id": "mid", "alive": False, "position": None},
                {"id": "support", "alive": True, "position": {"x": 49, "z": 48}},
            ],
            "visibleEnemies": [
                {"id": "red-jungle", "alive": True, "position": {"x": 55, "z": 54}}
            ],
            "objective": {
                "type": objective_type,
                "position": {"x": 55, "z": 55},
                "isCritical": critical,
                "teamCommitted": committed,
                "fightActive": fight_active,
                "secondsToResolution": seconds,
            },
        },
        "label": {
            "movement": {"waypoints": [{"x": 53, "z": 54}]},
            "matchWonByTeam": won,
            "objectiveSecuredByTeam": secured,
            "sourceType": "manually_reviewed_replay",
            "evidenceId": f"evidence-{match_id}-{movement_index}",
        },
        "metadata": {
            "coordinateMethod": "dataset-native x/z coordinates",
            "labelProvenance": "reviewed match result and objective event",
        },
    }


class MovementPipelineTests(unittest.TestCase):
    def test_validates_teammate_positions_and_dead_state(self):
        record = validate_record(movement_record(), 0)
        self.assertFalse(record["state"]["teammates"][2]["alive"])
        self.assertIsNone(record["state"]["teammates"][2]["position"])

    def test_alive_teammate_requires_position(self):
        record = movement_record()
        record["state"]["teammates"][0]["position"] = None
        with self.assertRaises(ValueError):
            validate_record(record, 0)

    def test_only_committed_critical_objective_windows_are_selected(self):
        records = [
            validate_record(movement_record(movement_index=0), 0),
            validate_record(movement_record(movement_index=1, objective_type="rift_herald"), 1),
            validate_record(movement_record(movement_index=2, committed=False), 2),
            validate_record(movement_record(movement_index=3, seconds=120), 3),
            validate_record(movement_record(movement_index=4, fight_active=False), 4),
        ]
        selected, exclusions = select_eligible_records(records)
        self.assertEqual([record["movementIndex"] for record in selected], [0])
        self.assertEqual(sum(exclusions.values()), 4)

    def test_training_is_mainly_wins_with_secured_objectives(self):
        records = [validate_record(movement_record(movement_index=0), 0)]
        records.extend(
            validate_record(
                movement_record(movement_index=index, won=False, secured=index % 2 == 0),
                index,
            )
            for index in range(1, 8)
        )
        balanced = balance_training_records(
            records, target_positive_fraction=0.8, seed=7
        )
        self.assertGreaterEqual(positive_fraction(balanced), 0.8)
        self.assertTrue(any(not item["label"]["matchWonByTeam"] for item in balanced))

    def test_outcomes_select_examples_but_never_enter_model_input(self):
        record = validate_record(movement_record(), 0)
        example = build_training_example(record)
        model_input = json.loads(example["messages"][1]["content"])
        self.assertNotIn("matchWonByTeam", model_input)
        self.assertNotIn("objectiveSecuredByTeam", model_input)
        self.assertEqual(len(model_input["teammates"]), 4)
        self.assertEqual(len(model_input["visibleEnemies"]), 1)
        self.assertTrue(example["metadata"]["positiveSelectionClass"])

    def test_group_split_never_crosses_match_boundaries(self):
        records = [
            validate_record(movement_record(match_id, movement_index=index), index)
            for index, match_id in enumerate(("a", "a", "b", "b", "c", "c"))
        ]
        train, evaluation = grouped_split(records, eval_fraction=0.34, seed=11)
        self.assertFalse(
            {record["matchId"] for record in train}
            & {record["matchId"] for record in evaluation}
        )
        self.assertEqual(records[0]["matchId"], record_order_key(records[0])[0])

    def test_group_split_reserves_a_dragon_match_for_evaluation(self):
        records = [
            validate_record(movement_record("baron-a", objective_type="baron_nashor"), 0),
            validate_record(movement_record("baron-b", objective_type="baron_nashor"), 1),
            validate_record(movement_record("dragon-a", objective_type="dragon"), 2),
        ]
        train, evaluation = grouped_split(
            records,
            eval_fraction=0.2,
            seed=1,
            required_eval_objective_types=DRAGON_OBJECTIVE_TYPES,
        )
        self.assertTrue(
            any(record["state"]["objective"]["type"] == "dragon" for record in evaluation)
        )
        self.assertFalse(
            {record["matchId"] for record in train}
            & {record["matchId"] for record in evaluation}
        )

    def test_scores_and_summarizes_heldout_dragon_prediction(self):
        prediction = {"movement": {"waypoints": [{"x": 56, "z": 55}]}}
        expected = {"movement": {"waypoints": [{"x": 53, "z": 51}]}}
        result = score_movement_prediction(prediction, expected)
        result["objectiveType"] = "elder_dragon"
        report = summarize_heldout_results([result])
        self.assertEqual(result["endpointError"], 5)
        self.assertEqual(report["dragonObjectiveFights"]["examples"], 1)
        self.assertEqual(report["dragonObjectiveFights"]["validMovementRate"], 1)


if __name__ == "__main__":
    unittest.main()

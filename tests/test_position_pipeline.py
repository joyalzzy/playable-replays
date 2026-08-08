import json
import unittest

from ml.position_pipeline import (
    build_training_example,
    grouped_split,
    score_position_prediction,
    summarize_results,
    validate_frame_pair,
)


def frame_pair(match_id="match-1", frame_index=0):
    players = [
        {
            "playerId": str(1000 + slot),
            "champion": f"champion-{slot}",
            "position": {"x": float(slot), "z": float(slot + 1)},
        }
        for slot in range(10)
    ]
    next_players = [
        {
            "playerId": player["playerId"],
            "position": {
                "x": player["position"]["x"] + 1,
                "z": player["position"]["z"] + 2,
            },
        }
        for player in players
    ]
    return {
        "matchId": match_id,
        "frameIndex": frame_index,
        "nextFrameIndex": frame_index + 1,
        "frameTimeSeconds": float(frame_index),
        "nextFrameTimeSeconds": float(frame_index + 1),
        "players": players,
        "nextPlayers": next_players,
        "metadata": {
            "datasetRepo": "maknee/league-of-legends-decoded-replay-packets",
            "datasetRevision": "revision",
            "datasetFile": "13_2/batch_001.jsonl.gz",
            "datasetFileSha256": "0" * 64,
            "coordinateMethod": "Gym game_state dataset-native x/z positions",
        },
    }


class PositionPipelineTests(unittest.TestCase):
    def test_requires_exactly_ten_players(self):
        record = frame_pair()
        record["players"].pop()
        with self.assertRaises(ValueError):
            validate_frame_pair(record, 0)

    def test_requires_same_players_in_both_frames(self):
        record = frame_pair()
        record["nextPlayers"][0]["playerId"] = "different"
        with self.assertRaises(ValueError):
            validate_frame_pair(record, 0)

    def test_model_input_and_target_each_contain_ten_players(self):
        record = validate_frame_pair(frame_pair(), 0)
        example = build_training_example(record)
        model_input = json.loads(example["messages"][1]["content"])
        target = json.loads(example["messages"][2]["content"])
        self.assertEqual(len(model_input["players"]), 10)
        self.assertEqual(len(target["players"]), 10)
        self.assertEqual(
            {player["playerId"] for player in model_input["players"]},
            {player["playerId"] for player in target["players"]},
        )

    def test_group_split_holds_out_whole_matches(self):
        records = [
            validate_frame_pair(frame_pair(match_id, index), index)
            for index, match_id in enumerate(("a", "a", "b", "b", "c", "c"))
        ]
        train, evaluation = grouped_split(records, eval_fraction=0.34, seed=4)
        self.assertFalse(
            {record["matchId"] for record in train}
            & {record["matchId"] for record in evaluation}
        )

    def test_scores_all_ten_positions(self):
        record = validate_frame_pair(frame_pair(), 0)
        expected = {
            "task": "ten_player_next_frame_position",
            "frameTimeSeconds": record["nextFrameTimeSeconds"],
            "players": record["nextPlayers"],
        }
        prediction = json.loads(json.dumps(expected))
        prediction["players"][0]["position"]["x"] += 10
        result = score_position_prediction(prediction, expected)
        report = summarize_results([result])
        self.assertTrue(result["validTenPlayerFrame"])
        self.assertEqual(result["meanPlayerPositionError"], 1)
        self.assertEqual(report["validTenPlayerFrameRate"], 1)


if __name__ == "__main__":
    unittest.main()

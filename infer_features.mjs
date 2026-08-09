#!/usr/bin/env node
import { readFile } from 'node:fs/promises';
import process from 'node:process';

const modelPath = new URL('./unit_policy_v2_carry_safety.json', import.meta.url);
const model = JSON.parse(await readFile(modelPath, 'utf8'));
const inputPath = process.argv[2];
if (!inputPath) {
  console.error('usage: node infer_features.mjs features.json');
  process.exit(2);
}
const input = JSON.parse(await readFile(inputPath, 'utf8'));
const features = Array.isArray(input) ? input : input.features;
if (!Array.isArray(features) || features.length !== model.featureNames.length) {
  throw new Error(`features must contain ${model.featureNames.length} numbers`);
}
const dot = (weights, xs) => weights.reduce((sum, weight, i) => sum + weight * xs[i], 0);
const logits = Object.fromEntries(model.actionOrder.map(action => [action, dot(model.actionWeights[action], features)]));
const maximum = Math.max(...Object.values(logits));
const expValues = Object.fromEntries(Object.entries(logits).map(([key, value]) => [key, Math.exp(value - maximum)]));
const total = Object.values(expValues).reduce((sum, value) => sum + value, 0);
const probabilities = Object.fromEntries(Object.entries(expValues).map(([key, value]) => [key, value / total]));
const action = model.actionOrder.reduce((best, candidate) => logits[candidate] > logits[best] ? candidate : best);
let dx = Math.tanh(dot(model.movementWeights.dx, features));
let dy = Math.tanh(dot(model.movementWeights.dy, features));
const magnitude = Math.hypot(dx, dy);
if (magnitude > 1) { dx /= magnitude; dy /= magnitude; }
console.log(JSON.stringify({ modelVersion: model.policyVersion, action, logits, probabilities, movementDelta: { dx, dy } }, null, 2));

# Min Score Picker

**Type:** `min-score-picker`

Selects the endpoint(s) with the lowest score calculated during the scoring phase.

## What it does

1.  Receives a list of `ScoredEndpoint` candidates.
2.  Shuffles the list in-place to ensure random tie-breaking when multiple endpoints share the same minimum score.
3.  Sorts the candidates by score in ascending order.
4.  Returns the bottom `maxNumOfEndpoints` candidates.

## Behavioral Intent

Counterpart to `max-score-picker` for objectives where a lower combined score is better. It is the picker for a multiplicative combination of raw cost signals (e.g. `data['p_token'] * data['bs']`), where the endpoint with the smallest product is the best target.

## Inputs consumed

- Consumes the list of `ScoredEndpoint` results from the scoring phase.

## Configuration

The plugin config supports:

- `maxNumOfEndpoints` (default 1)
  - The maximum number of endpoints to pick and return. Must be > 0. If more candidates are available than this limit, only the bottom subset is returned.

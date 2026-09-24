# CEL Combiner

**Type:** `cel-combiner`

Folds the per-scorer results of a scheduling profile into a single score per
endpoint by evaluating a [CEL](https://cel.dev/) expression once per endpoint.

## What it does

For each candidate endpoint, the configured expression is evaluated against two
map variables and the numeric result becomes that endpoint's combined score:

- `scores['scorer-name']` - the endpoint's output from the named scorer, in
  `[0, 1]` (the scorer chain's normalized outputs).
- `data['producer']['signal']` - raw magnitudes grouped by producer instance
  name (the part after `/` in the attribute's `DataKey`), plus two synthetic
  buckets: `data['metrics']` from the endpoint metrics snapshot and
  `data['custom']` for bare scalar attributes such as custom metrics.

The combined score is handed to the profile's picker. The combiner is
orientation-neutral: pair it with `max-score-picker` when higher is better and
`min-score-picker` when lower is better.

## Signals

Views are computed from the live values at `Combine` time; producers never
write combiner-specific attributes.

| Expression | Source |
|---|---|
| `data['<inflight-producer>']['tokens_uncached']` | request's uncached prefill tokens for this endpoint (`UncachedRequestTokens`) |
| `data['<inflight-producer>']['tokens_inflight']` | endpoint's tracked in-flight tokens (`InFlightLoad`) |
| `data['<inflight-producer>']['requests_inflight']` | endpoint's tracked in-flight requests (`InFlightLoad`) |
| `data['metrics']['requests_running']` | running requests |
| `data['metrics']['requests_waiting']` | waiting (queued) requests |
| `data['metrics']['cache_usage']` | KV cache usage fraction |
| `data['metrics']['cache_block_size']` | KV cache block size in tokens |
| `data['metrics']['cache_max_blocks']` | number of KV cache blocks |
| `data['metrics']['cache_max_tokens']` | KV cache max token capacity |
| `data['metrics']['models_max_active']` | max LoRA-adapted models resident |
| `data['<prefix-producer>']['blocks_matched']` | matched prefix blocks for this request (tier-weighted for the precise producer) |
| `data['<prefix-producer>']['blocks_total']` | total indexed prefix blocks |
| `data['<prefix-producer>']['tokens_cached']` | unweighted cached prefix tokens for this request |
| `data['<prefix-producer>']['tokens_block_size']` | prefix block size in tokens |
| `data['custom']['<attribute-key>']` | bare scalar attributes (custom metrics extraction) |

Signal names follow `resource_state`. Derived combinations such as total
batch size (`requests_running + requests_waiting`) belong in the expression,
not in the views.

Replace `<inflight-producer>` with the configured instance name of the
in-flight-load producer (its type name by default).

## Configuration

- `expression` (required) - the CEL expression. Must evaluate to a number.

```yaml
- type: cel-combiner
  parameters:
    expression: "scores['prefix-cache-scorer'] * scores['token-load-scorer']"
```

## Examples

Multiply two normalized scorer outputs (higher is better, use
`max-score-picker`):

```
scores['prefix-cache-scorer'] * scores['token-load-scorer']
```

Raw cost-signal product (lower is better, use `min-score-picker`); the
`+ 1` counts the incoming request and keeps idle instances comparable:

```
data['inflight-load-producer']['tokens_uncached'] * (data['metrics']['requests_running'] + data['metrics']['requests_waiting'] + 1.0)
```

Mix a normalized score with a raw signal:

```
scores['prefix-cache-scorer'] * (1.0 - data['metrics']['cache_usage'])
```

## Notes

- `scores[]` are normalized to `[0, 1]`; `data[]` are raw and may span any
  range. Mixing the two scales in one expression is intentional and up to the
  expression author. Multiplicative weight-cancellation only holds within a
  consistent-scale expression. Scorer `weight:` values are ignored by CEL
  expressions; the expression is the weighting.
- The expression is compiled once at startup; a compile error fails plugin
  construction.
- If evaluation errors for an endpoint (for example, the expression references a
  `data[]` key the endpoint does not have, or the result is not numeric), that
  endpoint is excluded from the result and can never win the pick. Missing keys
  are not validated at config load time; they surface at runtime.

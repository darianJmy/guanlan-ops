// Range-query samples may omit evaluation timestamps with no data.
// Explicit nulls stop the chart from drawing across those gaps.
export function prometheusSeriesPoints(samples, stepSeconds) {
  const points = []
  const stepMs = Number(stepSeconds) * 1000
  let previousTime
  for (const [timestamp, value] of samples || []) {
    const time = Number(timestamp) * 1000
    if (!Number.isFinite(time)) continue
    if (Number.isFinite(stepMs) && stepMs > 0 && previousTime !== undefined && time - previousTime > stepMs * 1.5) {
      points.push([previousTime + stepMs, null])
    }
    const number = value == null || value === '' ? NaN : Number(value)
    points.push([time, Number.isFinite(number) ? number : null])
    previousTime = time
  }
  return points
}

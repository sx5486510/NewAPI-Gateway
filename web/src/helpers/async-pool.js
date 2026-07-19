export const mapWithConcurrency = async (items, limit, worker) => {
  const results = new Array(items.length);
  let nextIndex = 0;
  const workerCount = Math.min(Math.max(1, limit), items.length);
  const runners = Array.from({ length: workerCount }, async () => {
    while (nextIndex < items.length) {
      const index = nextIndex++;
      try {
        results[index] = {
          status: 'fulfilled',
          value: await worker(items[index], index),
        };
      } catch (reason) {
        results[index] = { status: 'rejected', reason };
      }
    }
  });
  await Promise.all(runners);
  return results;
};

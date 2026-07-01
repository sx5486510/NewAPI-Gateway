const formatJson = (value) => JSON.stringify(value, null, 2);

const findEmbeddedJson = (text) => {
  const starts = [];
  for (let i = 0; i < text.length; i += 1) {
    if (text[i] === '{' || text[i] === '[') {
      starts.push(i);
    }
  }

  for (const start of starts) {
    for (let end = text.length; end > start; end -= 1) {
      const candidate = text.slice(start, end).trim();
      if (!candidate) {
        continue;
      }
      try {
        return {
          start,
          end,
          value: JSON.parse(candidate)
        };
      } catch (e) {
        // Keep scanning; upstream errors often wrap JSON with plain text.
      }
    }
  }

  return null;
};

export const formatTraceContent = (value) => {
  const text = String(value || '');
  if (!text.trim()) {
    return '-';
  }

  try {
    return formatJson(JSON.parse(text));
  } catch (e) {
    const embeddedJson = findEmbeddedJson(text);
    if (!embeddedJson) {
      return text;
    }

    const before = text.slice(0, embeddedJson.start).trim();
    const after = text.slice(embeddedJson.end).trim();
    return [before, formatJson(embeddedJson.value), after].filter(Boolean).join('\n');
  }
};

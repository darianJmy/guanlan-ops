const diagrams = [...document.querySelectorAll('.language-mermaid pre code, pre code.language-mermaid, pre code[data-lang="mermaid"]')];
if (diagrams.length) {
  try {
    const { default: mermaid } = await import('https://cdn.jsdelivr.net/npm/mermaid@11.4.1/dist/mermaid.esm.min.mjs');
    mermaid.initialize({ startOnLoad: false, securityLevel: 'strict', theme: 'neutral' });
    for (const [index, code] of diagrams.entries()) {
      try {
        const { svg } = await mermaid.render(`diagram-${index}`, code.textContent);
        const rendered = document.createElement('div');
        rendered.className = 'mermaid';
        rendered.innerHTML = svg;
        code.closest('pre').replaceWith(rendered);
      } catch (error) {
        console.warn('Diagram kept as source:', error);
      }
    }
  } catch (error) {
    console.warn('Diagram renderer unavailable; showing source:', error);
  }
}

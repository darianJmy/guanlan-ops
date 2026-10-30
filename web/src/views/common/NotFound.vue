<template>
  <section class="not-found" aria-labelledby="not-found-title">
    <div class="illustration" aria-hidden="true">
      <svg viewBox="0 0 640 350" fill="none" xmlns="http://www.w3.org/2000/svg">
        <ellipse cx="320" cy="174" rx="235" ry="145" class="halo" />
        <ellipse cx="320" cy="307" rx="198" ry="12" class="ground" />
        <path d="M83 228C28 117 203 54 416 83C598 108 629 215 506 259" class="orbit" stroke-width="1.5" stroke-dasharray="5 9" />
        <path d="M106 291H532M141 302H165M489 302H513" class="line" stroke-width="2" stroke-linecap="round" />
        <!-- Geometric numerals keep the illustration independent of installed fonts. -->
        <path d="M186 110L115 215H198M184 110V273" class="number-shadow" stroke-width="31" stroke-linecap="round" stroke-linejoin="round" />
        <path d="M186 100L115 205H198M184 100V263" class="number" stroke-width="29" stroke-linecap="round" stroke-linejoin="round" />
        <path d="M500 110L429 215H512M498 110V273" class="number-shadow" stroke-width="31" stroke-linecap="round" stroke-linejoin="round" />
        <path d="M500 100L429 205H512M498 100V263" class="number" stroke-width="29" stroke-linecap="round" stroke-linejoin="round" />
        <g class="probe">
          <path d="M291 231L281 265L301 269L310 241M348 231L360 263L341 270L330 241" class="probe-limb" stroke-width="12" stroke-linecap="round" stroke-linejoin="round" />
          <path d="M267 173L244 195L230 184M374 173L395 151L407 160" class="probe-limb" stroke-width="13" stroke-linecap="round" stroke-linejoin="round" />
          <path d="M320 112V83" class="probe-outline" stroke-width="5" stroke-linecap="round" />
          <circle cx="320" cy="77" r="8" class="signal" />
          <path d="M304 65C313 55 327 55 336 65M296 54C309 40 331 40 344 54" class="orbit" stroke-width="2" stroke-linecap="round" />
          <rect x="264" y="115" width="111" height="126" rx="45" class="probe-shell" stroke-width="3" />
          <rect x="276" y="137" width="87" height="58" rx="24" class="visor" />
          <path d="M288 149C302 143 321 143 337 146" stroke="white" stroke-opacity=".2" stroke-width="3" stroke-linecap="round" />
          <path d="M299 161V173M340 161V173" class="eyes" stroke-width="7" stroke-linecap="round" />
          <path d="M314 181Q320 177 326 181" class="eyes" stroke-width="2" stroke-linecap="round" />
          <circle cx="307" cy="216" r="4" class="signal" />
          <path d="M320 216H339" class="line" stroke-width="4" stroke-linecap="round" />
          <rect x="252" y="153" width="12" height="31" rx="6" class="probe-side" />
          <rect x="375" y="153" width="12" height="31" rx="6" class="probe-side" />
        </g>
        <g transform="rotate(12 422 79)">
          <rect x="400" y="55" width="44" height="41" rx="12" class="question-bubble" />
          <path d="M417 67C418 61 430 62 429 69C429 74 422 73 422 79" class="probe-outline" stroke-width="2.5" stroke-linecap="round" />
          <circle cx="422" cy="85" r="1.5" class="question-dot" />
        </g>
        <g class="spark" stroke-width="2" stroke-linecap="round">
          <path d="M119 77V89M113 83H125M529 156V168M523 162H535M238 57V65M234 61H242" />
        </g>
        <circle cx="89" cy="181" r="4" class="signal" />
        <circle cx="552" cy="230" r="5" class="ground" />
        <circle cx="221" cy="272" r="3" class="question-dot" />
      </svg>
    </div>
    <div class="not-found-copy">
      <h1 id="not-found-title">页面未找到</h1>
      <p>地址有误或页面已移除。</p>
      <router-link class="home-link" to="/dashboard">
        <svg viewBox="0 0 24 24" fill="none" aria-hidden="true"><path d="m9 5-7 7 7 7M3 12h18" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" /></svg>
        返回首页
      </router-link>
    </div>
  </section>
</template>

<style scoped>
.not-found { display: flex; min-height: calc(100dvh - 148px); box-sizing: border-box; align-items: center; justify-content: center; flex-direction: column; padding: 28px 16px 48px; text-align: center; }
.illustration { width: min(100%, 580px); }
.illustration svg { display: block; width: 100%; height: auto; }
.halo { fill: var(--ops-primary-soft); opacity: .65; }
.ground { fill: var(--ops-primary); opacity: .1; }
.orbit { stroke: var(--ops-primary); opacity: .3; }
.line { stroke: var(--ops-border); }
.number { stroke: color-mix(in srgb, var(--ops-primary) 28%, var(--ops-surface)); }
.number-shadow { stroke: var(--ops-primary); opacity: .09; }
.probe-shell { fill: var(--ops-surface); stroke: color-mix(in srgb, var(--ops-primary) 40%, var(--ops-border)); }
.probe-limb { stroke: color-mix(in srgb, var(--ops-primary) 50%, var(--ops-surface)); }
.probe-outline { stroke: var(--ops-primary); }
.probe-side { fill: var(--ops-primary); }
.visor { fill: #183d60; }
.eyes { stroke: #9ce6ff; }
.signal { fill: #f0b85e; }
.question-bubble { fill: var(--ops-surface); stroke: var(--ops-border); }
.question-dot { fill: var(--ops-primary); }
.spark { stroke: var(--ops-primary); opacity: .4; }
.probe { animation: float 5s ease-in-out infinite; }
.not-found-copy { margin-top: 2px; }
.not-found h1 { margin: 14px 0 12px; color: var(--ops-text); font-size: clamp(22px, 3vw, 28px); font-weight: 600; letter-spacing: 1px; }
.not-found p { margin: 0 0 26px; color: var(--ops-muted); font-size: 13px; line-height: 1.9; }
.home-link { display: inline-flex; align-items: center; justify-content: center; gap: 10px; min-height: 40px; padding: 0 24px; border-radius: var(--ops-radius); background: var(--ops-primary); color: var(--ops-on-primary); text-decoration: none; font-size: 13px; transition: box-shadow .2s, transform .2s; }
.home-link svg { width: 16px; height: 16px; }
.home-link:hover { transform: translateY(-1px); box-shadow: 0 5px 16px color-mix(in srgb, var(--ops-primary) 22%, transparent); }
.home-link:focus-visible { outline: 2px solid var(--ops-primary); outline-offset: 4px; }
@keyframes float { 0%, 100% { transform: translateY(0); } 50% { transform: translateY(-8px); } }
@media (max-width: 600px) { .not-found { min-height: 70dvh; padding: 28px 4px; }.not-found-copy { margin-top: 14px; } }
@media (prefers-reduced-motion: reduce) { .probe { animation: none; }.home-link { transition: none; } }
</style>

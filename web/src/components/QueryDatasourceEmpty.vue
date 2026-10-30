<template>
  <section class="query-empty" aria-labelledby="query-empty-title">
    <svg class="query-empty-art" viewBox="0 0 560 310" fill="none" aria-hidden="true" xmlns="http://www.w3.org/2000/svg">
      <ellipse cx="280" cy="151" rx="192" ry="126" class="halo" />
      <ellipse cx="284" cy="271" rx="168" ry="10" class="shadow" />
      <path d="M91 190C51 110 154 48 318 58C476 67 510 155 453 211" class="orbit" stroke-width="1.5" stroke-dasharray="4 8" />
      <!-- The database is waiting for a connection; the lens represents querying. -->
      <path d="M161 111V217C161 238 193 255 234 255C275 255 307 238 307 217V111" class="database" stroke-width="2.5" />
      <ellipse cx="234" cy="111" rx="73" ry="29" class="database-top" stroke-width="2.5" />
      <ellipse cx="234" cy="111" rx="49" ry="17" class="inner-ring" stroke-width="1.5" />
      <path d="M161 146C161 167 193 184 234 184C275 184 307 167 307 146M161 181C161 202 193 219 234 219C275 219 307 202 307 181" class="database-line" stroke-width="2" />
      <path d="M180 155L191 160M180 190L191 195M180 225L191 230" class="status-line" stroke-width="4" stroke-linecap="round" />
      <path d="M310 229H342C354 229 356 212 368 212H393" class="connection" stroke-width="2" stroke-linecap="round" stroke-dasharray="5 6" />
      <g class="search-lens">
        <path d="M389 173L431 216" class="lens-handle-shadow" stroke-width="23" stroke-linecap="round" />
        <path d="M387 170L429 213" class="lens-handle" stroke-width="17" stroke-linecap="round" />
        <circle cx="350" cy="130" r="64" class="lens-rim" stroke-width="3" />
        <circle cx="350" cy="130" r="52" class="lens-glass" stroke-width="2" />
        <path d="M317 100C325 92 336 87 347 87" class="lens-shine" stroke-width="5" stroke-linecap="round" />
        <path d="M320 135H333L342 119L354 147L364 130H382" class="signal" stroke-width="3" stroke-linejoin="round" stroke-linecap="round" />
      </g>
      <g transform="rotate(-9 127 85)">
        <rect x="105" y="65" width="44" height="38" rx="10" class="chip" stroke-width="1.5" />
        <path d="m121 76-7 8 7 8m12-16 7 8-7 8" class="database-line" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" />
      </g>
      <circle cx="435" cy="78" r="5" class="dot-warm" />
      <circle cx="107" cy="210" r="4" class="dot-blue" />
      <path d="M454 150V162M448 156H460M239 44V52M235 48H243" class="spark" stroke-width="2" stroke-linecap="round" />
      <path d="M119 269H443M143 281H167M391 281H410" class="baseline" stroke-width="2" stroke-linecap="round" />
    </svg>
    <h2 id="query-empty-title">还没有可查询的数据源</h2>
    <p>接入数据源，或联系管理员开通查询权限。</p>
    <router-link v-permission="['datasource:list']" class="connect-link" to="/integrations/sources">
      <span aria-hidden="true">＋</span>接入数据源
    </router-link>
  </section>
</template>

<style scoped>
.query-empty { display:flex; flex-direction:column; align-items:center; justify-content:center; min-height:calc(100dvh - 210px); box-sizing:border-box; padding:24px 16px 48px; text-align:center; }
.query-empty-art { display:block; width:min(100%,480px); height:auto; }
.halo { fill:var(--ops-primary-soft); opacity:.65; }
.shadow { fill:var(--ops-primary); opacity:.09; }
.orbit,.connection { stroke:var(--ops-primary); opacity:.3; }
.database { fill:var(--ops-surface); stroke:color-mix(in srgb,var(--ops-primary) 40%,var(--ops-border)); }
.database-top { fill:color-mix(in srgb,var(--ops-primary) 8%,var(--ops-surface)); stroke:color-mix(in srgb,var(--ops-primary) 40%,var(--ops-border)); }
.inner-ring { stroke:var(--ops-primary); opacity:.18; }
.database-line { stroke:color-mix(in srgb,var(--ops-primary) 45%,var(--ops-border)); }
.status-line { stroke:var(--ops-primary); opacity:.5; }
.lens-rim { fill:var(--ops-surface); stroke:var(--ops-primary); }
.lens-glass { fill:color-mix(in srgb,var(--ops-primary) 8%,var(--ops-surface)); stroke:color-mix(in srgb,var(--ops-primary) 25%,var(--ops-surface)); }
.lens-shine { stroke:var(--ops-surface); }
.lens-handle { stroke:var(--ops-primary); }
.lens-handle-shadow { stroke:var(--ops-primary); opacity:.12; }
.signal { stroke:var(--ops-primary); opacity:.65; }
.chip { fill:var(--ops-surface); stroke:var(--ops-border); }
.dot-warm { fill:#f0b85e; }
.dot-blue { fill:var(--ops-primary); opacity:.35; }
.spark { stroke:var(--ops-primary); opacity:.35; }
.baseline { stroke:var(--ops-border); }
.search-lens { animation:query-float 6s ease-in-out infinite; }
.query-empty h2 { margin:6px 0 10px; color:var(--ops-text); font-size:20px; font-weight:500; }
.query-empty p { margin:0 0 22px; color:var(--ops-muted); font-size:13px; line-height:1.7; }
.connect-link { display:inline-flex; align-items:center; justify-content:center; gap:8px; min-height:38px; padding:0 20px; color:var(--ops-on-primary); background:var(--ops-primary); border-radius:var(--ops-radius); text-decoration:none; font-size:13px; }
.connect-link>span { font-size:18px; }
.connect-link:hover { opacity:.9; }
.connect-link:focus-visible { outline:2px solid var(--ops-primary); outline-offset:3px; }
@keyframes query-float { 0%,100% { transform:translateY(0); } 50% { transform:translateY(-6px); } }
@media (max-width:600px) { .query-empty { min-height:60dvh; padding:20px 4px; }.query-empty h2 { font-size:18px; } }
@media (prefers-reduced-motion:reduce) { .search-lens { animation:none; } }
</style>

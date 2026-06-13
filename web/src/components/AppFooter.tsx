// A small, self-contained footer that credits the project.
//
// The version and git hash are injected at build time (see vite.config.ts
// `define`), so the footer shows the exact build without any runtime lookup.
const REPO_URL = 'https://github.com/lieyanc/FireVoiceBox'
const REPO_SLUG = 'lieyanc/FireVoiceBox'

// Official GitHub mark — inlined so we don't depend on a deprecated brand icon.
function GithubMark({ className }: { className?: string }) {
  return (
    <svg viewBox="0 0 16 16" className={className} fill="currentColor" aria-hidden>
      <path d="M8 0C3.58 0 0 3.58 0 8c0 3.54 2.29 6.53 5.47 7.59.4.07.55-.17.55-.38 0-.19-.01-.82-.01-1.49-2.01.37-2.53-.49-2.69-.94-.09-.23-.48-.94-.82-1.13-.28-.15-.68-.52-.01-.53.63-.01 1.08.58 1.23.82.72 1.21 1.87.87 2.33.66.07-.52.28-.87.51-1.07-1.78-.2-3.64-.89-3.64-3.95 0-.87.31-1.59.82-2.15-.08-.2-.36-1.02.08-2.12 0 0 .67-.21 2.2.82.64-.18 1.32-.27 2-.27.68 0 1.36.09 2 .27 1.53-1.04 2.2-.82 2.2-.82.44 1.1.16 1.92.08 2.12.51.56.82 1.27.82 2.15 0 3.07-1.87 3.75-3.65 3.95.29.25.54.73.54 1.48 0 1.07-.01 1.93-.01 2.2 0 .21.15.46.55.38A8.012 8.012 0 0016 8c0-4.42-3.58-8-8-8z" />
    </svg>
  )
}

export function AppFooter({ className = '' }: { className?: string }) {
  return (
    <footer
      className={`flex flex-wrap items-center justify-center gap-1.5 text-xs text-muted-foreground ${className}`}
    >
      <a
        href={REPO_URL}
        target="_blank"
        rel="noopener noreferrer"
        className="inline-flex items-center gap-1.5 rounded px-0.5 transition-colors hover:text-foreground"
      >
        <GithubMark className="h-3.5 w-3.5" />
        <span>{REPO_SLUG}</span>
      </a>
      <span aria-hidden className="text-muted-foreground/40">
        ·
      </span>
      <span className="inline-flex items-baseline gap-1 tabular-nums">
        v{__APP_VERSION__}
        <span className="font-mono text-[11px] text-muted-foreground/60">{__APP_GIT_HASH__}</span>
      </span>
    </footer>
  )
}

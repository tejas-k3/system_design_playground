export const Stars = ({ n, size = 16 }: { n: number; size?: number }) => (
  <span className="stars" aria-label={`${n} of 3 stars`}>
    {[1, 2, 3].map((i) => (
      <svg key={i} width={size} height={size} viewBox="0 0 24 24" className={i <= n ? 'on' : ''} aria-hidden>
        <path d="M12 3l2.7 5.6 6.1.8-4.5 4.3 1.1 6.1L12 17l-5.4 2.8 1.1-6.1L3.2 9.4l6.1-.8z" />
      </svg>
    ))}
  </span>
)

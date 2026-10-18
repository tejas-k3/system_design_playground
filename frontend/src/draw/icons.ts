// Small hand-built glyphs drawn on a 24x24 grid centred at (0,0).

type Draw = (c: CanvasRenderingContext2D) => void

const stroke = (c: CanvasRenderingContext2D, f: () => void) => {
  c.beginPath()
  f()
  c.stroke()
}

const ICONS: Record<string, Draw> = {
  client: (c) => {
    stroke(c, () => c.arc(0, -5, 4.5, 0, Math.PI * 2))
    stroke(c, () => {
      c.moveTo(-9, 10)
      c.quadraticCurveTo(-9, 1, 0, 1)
      c.quadraticCurveTo(9, 1, 9, 10)
    })
  },
  loadBalancer: (c) => {
    stroke(c, () => {
      c.moveTo(-10, 0)
      c.lineTo(-2, 0)
      c.moveTo(-2, 0)
      c.lineTo(8, -8)
      c.moveTo(-2, 0)
      c.lineTo(8, 0)
      c.moveTo(-2, 0)
      c.lineTo(8, 8)
    })
    for (const y of [-8, 0, 8]) stroke(c, () => c.arc(10, y, 1.8, 0, Math.PI * 2))
  },
  appServer: (c) => {
    for (const y of [-9, 1]) {
      stroke(c, () => c.roundRect(-10, y, 20, 8, 2))
      stroke(c, () => c.arc(-5.5, y + 4, 0.9, 0, Math.PI * 2))
    }
    stroke(c, () => {
      c.moveTo(-3, -5)
      c.lineTo(6, -5)
      c.moveTo(-3, 5)
      c.lineTo(6, 5)
    })
  },
  cache: (c) => {
    stroke(c, () => {
      c.moveTo(3, -11)
      c.lineTo(-6, 2)
      c.lineTo(0, 2)
      c.lineTo(-3, 11)
      c.lineTo(7, -3)
      c.lineTo(1, -3)
      c.closePath()
    })
  },
  database: (c) => {
    stroke(c, () => c.ellipse(0, -7, 9, 3.5, 0, 0, Math.PI * 2))
    stroke(c, () => {
      c.moveTo(-9, -7)
      c.lineTo(-9, 7)
      c.ellipse(0, 7, 9, 3.5, 0, Math.PI, 0, true)
      c.lineTo(9, -7)
    })
    stroke(c, () => c.ellipse(0, 0, 9, 3.5, 0, 0, Math.PI))
  },
  readReplica: (c) => {
    c.save()
    c.translate(-2, -2)
    ICONS.database(c)
    c.restore()
    stroke(c, () => {
      c.moveTo(6, 8)
      c.lineTo(11, 8)
      c.lineTo(11, 3)
    })
  },
  queue: (c) => {
    for (const x of [-10, -3, 4]) stroke(c, () => c.rect(x, -4, 6, 8))
    stroke(c, () => {
      c.moveTo(-10, 9)
      c.lineTo(10, 9)
      c.moveTo(7, 6.5)
      c.lineTo(10, 9)
      c.lineTo(7, 11.5)
    })
  },
  worker: (c) => {
    stroke(c, () => c.arc(0, 0, 4.5, 0, Math.PI * 2))
    stroke(c, () => {
      for (let i = 0; i < 8; i++) {
        const a = (i * Math.PI) / 4
        c.moveTo(Math.cos(a) * 7, Math.sin(a) * 7)
        c.lineTo(Math.cos(a) * 10.5, Math.sin(a) * 10.5)
      }
    })
  },
  cdn: (c) => {
    stroke(c, () => c.arc(0, 0, 10, 0, Math.PI * 2))
    stroke(c, () => c.ellipse(0, 0, 4.5, 10, 0, 0, Math.PI * 2))
    stroke(c, () => {
      c.moveTo(-10, 0)
      c.lineTo(10, 0)
    })
  },
  rateLimiter: (c) => {
    stroke(c, () => {
      c.moveTo(-10, -9)
      c.lineTo(10, -9)
      c.lineTo(2, 0)
      c.lineTo(2, 9)
      c.lineTo(-2, 7)
      c.lineTo(-2, 0)
      c.closePath()
    })
  },
  shardRouter: (c) => {
    stroke(c, () => {
      c.moveTo(0, 10)
      c.lineTo(0, 0)
      c.lineTo(-8, -8)
      c.moveTo(0, 0)
      c.lineTo(8, -8)
    })
    for (const x of [-8, 8]) stroke(c, () => c.arc(x, -9, 2, 0, Math.PI * 2))
  },
  objectStore: (c) => {
    // a bucket: a tapered tub with an elliptical rim
    stroke(c, () => {
      c.ellipse(0, -7, 10, 3.5, 0, 0, Math.PI * 2)
      c.moveTo(-10, -7)
      c.lineTo(-7, 9)
      c.lineTo(7, 9)
      c.lineTo(10, -7)
    })
    stroke(c, () => {
      c.moveTo(-8.6, 0)
      c.quadraticCurveTo(0, 3.5, 8.6, 0)
    })
  },
  externalApi: (c) => {
    stroke(c, () => {
      c.arc(-4, 2, 5, Math.PI * 0.5, Math.PI * 1.5)
      c.arc(1, -3, 6, Math.PI, Math.PI * 1.9)
      c.arc(6, 3, 4.5, Math.PI * 1.5, Math.PI * 0.5)
      c.closePath()
    })
  },
  redis: (c) => {
    for (const y of [-6, 0, 6]) {
      stroke(c, () => {
        c.moveTo(-10, y)
        c.lineTo(0, y - 4)
        c.lineTo(10, y)
        c.lineTo(0, y + 4)
        c.closePath()
      })
    }
  },
}

export function drawIcon(c: CanvasRenderingContext2D, kind: string, x: number, y: number, color: string, scale = 1): void {
  const f = ICONS[kind]
  if (!f) return
  c.save()
  c.translate(x, y)
  c.scale(scale, scale)
  c.strokeStyle = color
  c.lineWidth = 1.8
  c.lineCap = 'round'
  c.lineJoin = 'round'
  f(c)
  c.restore()
}

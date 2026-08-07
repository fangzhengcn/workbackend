<!--
  登录页动态背景：挂谷（Kakeya）猜想，左右两幕。

  两幕分别对应这个问题的「经典形态」与「现代形态」，都不是随机装饰，
  几何是算出来的：

  ── 左：针在三尖瓣线内转身 ────────────────────────────────
    三尖瓣线   P(u) = ( 2a·cos u + a·cos 2u ,  2a·sin u − a·sin 2u )
    针         P(s) 与 P(s+π) 的连线

  该弦有三个可验证性质：长度恒为 4a、方向角恒为 π+s（故 s 走 π 针正好
  转满 180°）、且与曲线相切于 P(−2s)。改动系数会让切线关系失效、针穿出边界。
  注意切点是 P(−2s) 而非 P(−s/2)：后者不在弦上（验证时用共线残差可判别）。

  ── 右：三维 δ-管束 ──────────────────────────────────────
  挂谷集的现代表述：每个方向都有一根单位针，即「方向球上每点一根管」。
  这也是 2025 年 Wang–Zahl 证明三维猜想所用的 δ-管语言。

  方向取样用**半球**上的 Fibonacci（黄金角）序列，两点都关键：
  - z 必须均匀（而非 φ 均匀），否则方向在两极堆积：
    朴素 (θ,φ) 网格的最近邻间距离散度 cv≈0.475，Fibonacci 仅 0.021。
  - 取半球而非整球：针的方向是**无向**的（d 与 −d 是同一根针），
    整球取样会出现对径重合的管（实测最坏 |dot|=0.9993，两根管重叠着画，
    白费开销），半球最坏 0.979 且无向间距均匀度好近三倍。

  ── 渲染 ────────────────────────────────────────────────
  两幕共用一个 canvas，各自有：
  1. 静态层（离屏预渲染一次）：三尖瓣线轮廓 + 切线族。切线族的包络本身
     就勾出三尖瓣线 —— 这是该图形作为「包络」的来历。近百条线一帧不变，
     逐帧重画是白烧 CPU。
  2. 拖尾层（离屏，逐帧淡出）：针扫过的轨迹，光绘效果。
  3. 动态层：当前的针、切点、旋转的管束、网格、尘埃。
-->
<template>
  <canvas ref="canvasRef" class="kakeya-canvas" aria-hidden="true" />
</template>

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'

const canvasRef = ref<HTMLCanvasElement | null>(null)

/** 设备像素比上限。视网膜屏下 dpr 可达 3，像素数按平方增长，
 *  背景动画不值得为此多烧一倍 GPU，封顶 2 视觉上已无差别。 */
const DPR_CAP = 2
const TAU = Math.PI * 2
/** 黄金角，Fibonacci 球面取样用 */
const GOLDEN_ANGLE = Math.PI * (3 - Math.sqrt(5))
/** δ-管数量 */
const TUBE_COUNT = 48
/** 双幕布局所需的最小视口宽度。窄于此只保留左幕（针），
 *  否则两幕挤在一起谁都看不清。 */
const TWO_SCENE_MIN_WIDTH = 1024

/** 三尖瓣线上参数 u 处的点 */
function deltoidPoint(u: number, a: number): [number, number] {
  return [2 * a * Math.cos(u) + a * Math.cos(2 * u), 2 * a * Math.sin(u) - a * Math.sin(2 * u)]
}

/** 参数 s 对应的针（弦 P(s)–P(s+π)），返回两端点 */
function needleAt(s: number, a: number): [number, number, number, number] {
  const [x1, y1] = deltoidPoint(s, a)
  const [x2, y2] = deltoidPoint(s + Math.PI, a)
  return [x1, y1, x2, y2]
}

/** 半球 Fibonacci 取样，返回 TUBE_COUNT 个近似均匀的无向方向 */
function buildDirections(count: number): [number, number, number][] {
  const out: [number, number, number][] = []
  for (let i = 0; i < count; i++) {
    const z = (i + 0.5) / count // 半球：z 在 (0,1] 均匀
    const r = Math.sqrt(Math.max(0, 1 - z * z))
    const th = i * GOLDEN_ANGLE
    out.push([r * Math.cos(th), r * Math.sin(th), z])
  }
  return out
}

type Dust = { x: number; y: number; r: number; vx: number; vy: number; a: number }

let raf = 0
let disposed = false
/** 卸载时要执行的清理动作。onMounted 中途 return 时也能安全调用。 */
let cleanup = () => {}

onBeforeUnmount(() => {
  disposed = true
  if (raf) cancelAnimationFrame(raf)
  raf = 0
  cleanup()
})

onMounted(() => {
  const canvas = canvasRef.value
  if (!canvas) return
  const ctx = canvas.getContext('2d')
  if (!ctx) return // 极老的浏览器：静默退化为纯 CSS 底色

  // 尊重系统的「减弱动效」设置：前庭功能敏感的用户会因大面积持续运动不适。
  // 这里不是关掉画面，而是只画一帧静态构图 —— 信息量一样，只是不动。
  const reduceMotion = window.matchMedia('(prefers-reduced-motion: reduce)').matches

  let width = 0
  let height = 0
  let dprValue = 1

  // 左幕（针 + 三尖瓣线）
  let scaleA = 1
  let needleCx = 0
  let needleCy = 0
  // 右幕（δ-管束）
  let twoScene = false
  let tubeCx = 0
  let tubeCy = 0
  let tubeR = 1

  const directions = buildDirections(TUBE_COUNT)

  const staticLayer = document.createElement('canvas')
  const staticCtx = staticLayer.getContext('2d')
  const traceLayer = document.createElement('canvas')
  const traceCtx = traceLayer.getContext('2d')

  let dust: Dust[] = []

  // 鼠标视差：只存目标值，逐帧向它插值，避免指针抖动直接抖画面
  let pointerX = 0
  let pointerY = 0
  let parallaxX = 0
  let parallaxY = 0

  function buildStaticLayer() {
    if (!staticCtx) return
    staticCtx.setTransform(1, 0, 0, 1, 0, 0)
    staticCtx.clearRect(0, 0, staticLayer.width, staticLayer.height)
    staticCtx.setTransform(dprValue, 0, 0, dprValue, 0, 0)
    staticCtx.save()
    staticCtx.translate(needleCx, needleCy)

    // 切线族：每条都是一个合法的「针」位置，密集叠加后包络出三尖瓣线
    const FAMILY = 96
    for (let i = 0; i < FAMILY; i++) {
      const s = (i / FAMILY) * TAU
      const [x1, y1, x2, y2] = needleAt(s, scaleA)
      // 冷暖交替，让包络出现层次。浅底下用中低亮度的青/橙，避免糊成一片。
      staticCtx.strokeStyle = i % 2 ? 'rgba(56, 132, 255, 0.20)' : 'rgba(255, 122, 60, 0.14)'
      staticCtx.lineWidth = 1
      staticCtx.beginPath()
      staticCtx.moveTo(x1, y1)
      staticCtx.lineTo(x2, y2)
      staticCtx.stroke()
    }

    // 三尖瓣线轮廓本体，压在切线族之上
    staticCtx.beginPath()
    const STEPS = 720
    for (let i = 0; i <= STEPS; i++) {
      const [x, y] = deltoidPoint((i / STEPS) * TAU, scaleA)
      if (i === 0) staticCtx.moveTo(x, y)
      else staticCtx.lineTo(x, y)
    }
    staticCtx.closePath()
    staticCtx.strokeStyle = 'rgba(120, 190, 255, 0.75)'
    staticCtx.lineWidth = 1.5
    staticCtx.shadowColor = 'rgba(80, 160, 255, 0.8)'
    staticCtx.shadowBlur = 14
    staticCtx.stroke()
    staticCtx.shadowBlur = 0
    staticCtx.restore()
  }

  function resize() {
    const canvasEl = canvasRef.value
    if (!canvasEl) return
    dprValue = Math.min(window.devicePixelRatio || 1, DPR_CAP)
    // 用视口尺寸而非 clientWidth：背景是 position:fixed 铺满视口的，
    // 且首帧布局未完成时 clientWidth 可能为 0，会画出一片空白。
    width = window.innerWidth
    height = window.innerHeight

    for (const layer of [canvasEl, staticLayer, traceLayer]) {
      layer.width = Math.round(width * dprValue)
      layer.height = Math.round(height * dprValue)
    }
    canvasEl.style.width = `${width}px`
    canvasEl.style.height = `${height}px`

    twoScene = width >= TWO_SCENE_MIN_WIDTH

    if (twoScene) {
      // 双幕：左幕让给针，右幕放管束，中间留给登录卡片。
      // 卡片宽 380 居中，故其左边界在 0.5*width-190 —— 左幕圆心取 0.22*width
      // 才能让三尖瓣线整体落在卡片左侧而不被压住。
      needleCx = width * 0.24
      needleCy = height * 0.5
      // 三尖瓣线外接圆半径为 3a。这里按「可用半宽」定 a：
      // 不能超出左边界，也不能伸到卡片下面。
      const half = Math.min(needleCx * 0.86, height * 0.4)
      scaleA = half / 3
      tubeCx = width * 0.78
      tubeCy = height * 0.5
      tubeR = Math.min(width * 0.19, height * 0.4)
    } else {
      // 单幕：窄屏放不下两幕，只保留左幕（针）。
      // 但**不能**退回居中构图 —— 那样三尖瓣线又整体落到卡片背后，
      // 正是要避免的情形（实测 1000px 宽下卡片区域亮度占比高达 47%）。
      // 故把图形推到左上角、缩小尺寸，让它从卡片的左侧与上方绕过去。
      needleCx = width * 0.3
      needleCy = height * 0.3
      scaleA = Math.min(width * 0.44, height * 0.3) / 3
      tubeR = 0
    }

    if (traceCtx) {
      traceCtx.setTransform(1, 0, 0, 1, 0, 0)
      traceCtx.clearRect(0, 0, traceLayer.width, traceLayer.height)
    }

    // 尘埃按面积撒点，保证不同屏幕上的疏密感一致
    const count = Math.round(Math.min(70, (width * height) / 30000))
    dust = Array.from({ length: count }, () => ({
      x: Math.random() * width,
      y: Math.random() * height,
      r: 0.4 + Math.random() * 1.3,
      vx: (Math.random() - 0.5) * 0.12,
      vy: -0.05 - Math.random() * 0.14,
      a: 0.12 + Math.random() * 0.3,
    }))

    buildStaticLayer()
  }

  /** 背景底色：科技蓝灰渐变 + 中心光晕。比原先的深空色提亮约三档，
   *  科技感靠网格与青色高光体现，而非靠压暗。 */
  function paintBackdrop() {
    if (!ctx) return
    const g = ctx.createLinearGradient(0, 0, width, height)
    g.addColorStop(0, '#1b2540')
    g.addColorStop(0.5, '#223052')
    g.addColorStop(1, '#26355c')
    ctx.fillStyle = g
    ctx.fillRect(0, 0, width, height)

    const glow = ctx.createRadialGradient(
      width * 0.5,
      height * 0.5,
      0,
      width * 0.5,
      height * 0.5,
      Math.max(width, height) * 0.75,
    )
    glow.addColorStop(0, 'rgba(90, 140, 230, 0.20)')
    glow.addColorStop(0.6, 'rgba(60, 100, 190, 0.07)')
    glow.addColorStop(1, 'rgba(0, 0, 0, 0)')
    ctx.fillStyle = glow
    ctx.fillRect(0, 0, width, height)
  }

  /** 极淡的技术网格，提供「仪表盘 / 蓝图」的科技底纹 */
  function drawGrid() {
    if (!ctx) return
    const STEP = 46
    ctx.save()
    ctx.strokeStyle = 'rgba(140, 180, 255, 0.055)'
    ctx.lineWidth = 1
    ctx.beginPath()
    // 半像素偏移让 1px 线落在像素中心，不然会被插值成 2px 灰线
    for (let x = ((parallaxX * 0.4) % STEP) - STEP; x <= width; x += STEP) {
      ctx.moveTo(Math.round(x) + 0.5, 0)
      ctx.lineTo(Math.round(x) + 0.5, height)
    }
    for (let y = ((parallaxY * 0.4) % STEP) - STEP; y <= height; y += STEP) {
      ctx.moveTo(0, Math.round(y) + 0.5)
      ctx.lineTo(width, Math.round(y) + 0.5)
    }
    ctx.stroke()
    ctx.restore()
  }

  function drawDust(dt: number) {
    if (!ctx) return
    ctx.save()
    for (const p of dust) {
      p.x += p.vx * dt
      p.y += p.vy * dt
      // 环绕出界，维持恒定密度
      if (p.y < -5) p.y = height + 5
      if (p.x < -5) p.x = width + 5
      else if (p.x > width + 5) p.x = -5
      ctx.globalAlpha = p.a
      ctx.fillStyle = '#dcebff'
      ctx.beginPath()
      ctx.arc(p.x, p.y, p.r, 0, TAU)
      ctx.fill()
    }
    ctx.restore()
  }

  /** 把当前的针叠进拖尾层，并让旧痕迹整体淡出 */
  function accumulateTrace(s: number) {
    if (!traceCtx) return
    traceCtx.setTransform(1, 0, 0, 1, 0, 0)
    // destination-out 以极低 alpha 逐帧擦除，等效指数衰减：
    // 0.006 对应半衰期约两秒，拖尾足够长又不会糊成一片。
    traceCtx.globalCompositeOperation = 'destination-out'
    traceCtx.fillStyle = 'rgba(0, 0, 0, 0.006)'
    traceCtx.fillRect(0, 0, traceLayer.width, traceLayer.height)

    traceCtx.globalCompositeOperation = 'lighter'
    traceCtx.setTransform(dprValue, 0, 0, dprValue, 0, 0)
    traceCtx.translate(needleCx, needleCy)
    const [x1, y1, x2, y2] = needleAt(s, scaleA)
    const grad = traceCtx.createLinearGradient(x1, y1, x2, y2)
    grad.addColorStop(0, 'rgba(90, 160, 255, 0.34)')
    grad.addColorStop(0.5, 'rgba(190, 225, 255, 0.42)')
    grad.addColorStop(1, 'rgba(255, 150, 90, 0.32)')
    traceCtx.strokeStyle = grad
    traceCtx.lineWidth = 1.3
    traceCtx.beginPath()
    traceCtx.moveTo(x1, y1)
    traceCtx.lineTo(x2, y2)
    traceCtx.stroke()
  }

  /** 当前这一根针：主体 + 两端高光 + 切点 */
  function drawNeedle(s: number) {
    if (!ctx) return
    ctx.save()
    ctx.translate(needleCx + parallaxX, needleCy + parallaxY)

    const [x1, y1, x2, y2] = needleAt(s, scaleA)

    const grad = ctx.createLinearGradient(x1, y1, x2, y2)
    grad.addColorStop(0, '#6ea8ff')
    grad.addColorStop(0.5, '#f2f8ff')
    grad.addColorStop(1, '#ff9a5c')
    ctx.strokeStyle = grad
    ctx.lineWidth = 2.6
    ctx.lineCap = 'round'
    ctx.shadowColor = 'rgba(120, 180, 255, 0.95)'
    ctx.shadowBlur = 16
    ctx.beginPath()
    ctx.moveTo(x1, y1)
    ctx.lineTo(x2, y2)
    ctx.stroke()
    ctx.shadowBlur = 0

    for (const [px, py, color] of [
      [x1, y1, '#9cc4ff'],
      [x2, y2, '#ffb98a'],
    ] as const) {
      ctx.fillStyle = color
      ctx.shadowColor = color
      ctx.shadowBlur = 12
      ctx.beginPath()
      ctx.arc(px, py, 3, 0, TAU)
      ctx.fill()
    }
    ctx.shadowBlur = 0

    // 切点 P(−2s)：针始终与曲线相切于此，标出来才看得出「贴着边界转」
    const [tx, ty] = deltoidPoint(-2 * s, scaleA)
    ctx.fillStyle = 'rgba(255, 255, 255, 0.92)'
    ctx.shadowColor = 'rgba(255, 225, 190, 0.9)'
    ctx.shadowBlur = 14
    ctx.beginPath()
    ctx.arc(tx, ty, 2.2, 0, TAU)
    ctx.fill()
    ctx.shadowBlur = 0

    ctx.restore()
  }

  /**
   * 右幕：三维 δ-管束。挂谷集的现代表述是「每个方向都有一根单位针」，
   * 故这里给每个方向画一根定向短管，整体绕 Y 轴缓慢自转。
   * 用等距投影（正交投影 + 固定俯角），无需透视除法，也无近平面裁剪问题。
   *
   * 关键：管不能都画成过原点的直径 —— 那样 48 根线全交于一点，
   * 投影出来是个对称的「烟花」，完全看不出三维结构。真正的 δ-管束是
   * 各自带偏移的短线段，故给每根管一个沿自身方向的固定偏移中心。
   */
  function drawTubes(time: number) {
    if (!ctx || !twoScene) return
    const yaw = time * 0.00013
    const PITCH = 0.42
    const cosP = Math.cos(PITCH)
    const sinP = Math.sin(PITCH)
    const HALF = 0.34 // 管半长（相对球半径）

    ctx.save()
    ctx.translate(tubeCx + parallaxX * 1.4, tubeCy + parallaxY * 1.4)
    ctx.globalCompositeOperation = 'lighter'

    // 投影一个三维点
    const project = (x: number, y: number, z: number) => {
      const rx = x * Math.cos(yaw) + z * Math.sin(yaw)
      const rz = -x * Math.sin(yaw) + z * Math.cos(yaw)
      return {
        sx: rx * tubeR,
        sy: (y * cosP - rz * sinP) * tubeR,
        depth: y * sinP + rz * cosP,
      }
    }

    // 每根管：中心沿另一条「错开」的方向偏移，管体沿自身方向延伸
    const segs = directions.map(([dx, dy, dz], i) => {
      // 偏移中心取自序列里另一个方向，保证管彼此错开而不共点
      const [ox, oy, oz] = directions[(i * 7 + 11) % directions.length]
      const k = 0.55 // 偏移幅度
      const a = project(ox * k - dx * HALF, oy * k - dy * HALF, oz * k - dz * HALF)
      const b = project(ox * k + dx * HALF, oy * k + dy * HALF, oz * k + dz * HALF)
      return { a, b, depth: (a.depth + b.depth) / 2 }
    })
    // 远的先画 —— 否则远管会盖住近管，立体感全无
    segs.sort((p, q) => p.depth - q.depth)

    for (const seg of segs) {
      // depth ∈ [-1,1] → 近的更亮更粗，制造纵深
      const t = (seg.depth + 1) / 2
      const alpha = 0.22 + 0.6 * t
      ctx.strokeStyle = `rgba(${Math.round(120 + 75 * t)}, ${Math.round(190 + 45 * t)}, 255, ${alpha.toFixed(3)})`
      ctx.lineWidth = 1.2 + 2.4 * t
      ctx.lineCap = 'round'
      ctx.beginPath()
      ctx.moveTo(seg.a.sx, seg.a.sy)
      ctx.lineTo(seg.b.sx, seg.b.sy)
      ctx.stroke()
    }

    // 方向球的线框：赤道 + 两条经线，暗示这些管取自同一个方向球。
    // 没有它，管束看上去只是一团散线，看不出「方向」这层含义。
    ctx.globalCompositeOperation = 'source-over'
    ctx.strokeStyle = 'rgba(150, 200, 255, 0.20)'
    ctx.lineWidth = 1
    ctx.beginPath()
    ctx.ellipse(0, 0, tubeR, tubeR * sinP, 0, 0, TAU)
    ctx.stroke()
    // 经线随自转变宽变窄，强化「球在转」的感知
    for (const phase of [0, Math.PI / 2]) {
      const rx = Math.abs(Math.cos(yaw + phase)) * tubeR
      ctx.strokeStyle = 'rgba(150, 200, 255, 0.13)'
      ctx.beginPath()
      ctx.ellipse(0, 0, rx, tubeR, 0, 0, TAU)
      ctx.stroke()
    }

    ctx.restore()
  }

  function renderFrame(s: number, dt: number, time: number) {
    if (!ctx) return
    ctx.setTransform(1, 0, 0, 1, 0, 0)
    ctx.scale(dprValue, dprValue)

    paintBackdrop()
    drawGrid()
    drawDust(dt)

    // 视差插值（0.05 的跟随系数：明显但不跟手，像有惯性）
    parallaxX += (pointerX - parallaxX) * 0.05
    parallaxY += (pointerY - parallaxY) * 0.05

    ctx.save()
    ctx.globalCompositeOperation = 'lighter'
    // 静态层与拖尾层已含 dpr 变换，这里按 CSS 像素贴图
    ctx.drawImage(staticLayer, parallaxX, parallaxY, width, height)
    ctx.drawImage(traceLayer, parallaxX, parallaxY, width, height)
    ctx.restore()

    drawNeedle(s)
    drawTubes(time)
  }

  let s = 0
  let last = 0

  function loop(time: number) {
    if (disposed) return
    // dt 以 16.7ms 为 1，掉帧时运动速度不变；首帧 last=0 会算出巨大 dt，
    // 故第一帧钳到 1，否则尘埃会瞬间飞出屏幕。
    const dt = last ? Math.min((time - last) / 16.7, 3) : 1
    last = time
    s += 0.0035 * dt // 针转速：约 15 秒转满 180°，慢到能看清是「转身」
    accumulateTrace(s)
    renderFrame(s, dt, time)
    raf = requestAnimationFrame(loop)
  }

  function onPointerMove(e: PointerEvent) {
    // 归一化到 ±12px 的偏移量
    pointerX = ((e.clientX / window.innerWidth) * 2 - 1) * 12
    pointerY = ((e.clientY / window.innerHeight) * 2 - 1) * 12
  }

  // 标签页切到后台时停掉 rAF。浏览器通常会自行降频，但不保证暂停，
  // 而这个动画在后台毫无意义 —— 白耗电池。
  function onVisibility() {
    if (document.hidden) {
      if (raf) cancelAnimationFrame(raf)
      raf = 0
    } else if (!raf && !reduceMotion && !disposed) {
      last = 0 // 重置计时，避免用离开前的时间戳算出巨大 dt
      raf = requestAnimationFrame(loop)
    }
  }

  resize()
  window.addEventListener('resize', resize)
  document.addEventListener('visibilitychange', onVisibility)

  if (reduceMotion) {
    renderFrame(0.6, 1, 0) // 取一个构图好看的角度作静态帧
  } else {
    window.addEventListener('pointermove', onPointerMove)
    raf = requestAnimationFrame(loop)
  }

  cleanup = () => {
    window.removeEventListener('resize', resize)
    document.removeEventListener('visibilitychange', onVisibility)
    window.removeEventListener('pointermove', onPointerMove)
  }
})
</script>

<style scoped>
.kakeya-canvas {
  position: fixed;
  inset: 0;
  width: 100%;
  height: 100%;
  /* 背景是纯装饰，不能挡住表单的点击与选择 */
  pointer-events: none;
  /* canvas 尚未画出第一帧时先垫一层同色底，避免白屏闪一下 */
  background: #1b2540;
}
</style>

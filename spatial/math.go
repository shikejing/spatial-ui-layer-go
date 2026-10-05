package spatial

import "math"

// Vec3 是三维向量。
type Vec3 struct {
	X, Y, Z float64
}

// Vec2 是二维尺寸/坐标。
type Vec2 struct {
	X, Y float64
}

// Quat 是单位四元数表示的旋转。
type Quat struct {
	W, X, Y, Z float64
}

// Pose 是位置加朝向。
type Pose struct {
	Position    Vec3
	Orientation Quat
}

// Ray 是一条射线。
type Ray struct {
	Origin    Vec3
	Direction Vec3 // 必须为单位向量
}

func Add(a, b Vec3) Vec3           { return Vec3{a.X + b.X, a.Y + b.Y, a.Z + b.Z} }
func Sub(a, b Vec3) Vec3           { return Vec3{a.X - b.X, a.Y - b.Y, a.Z - b.Z} }
func Scale(a Vec3, s float64) Vec3 { return Vec3{a.X * s, a.Y * s, a.Z * s} }
func Dot(a, b Vec3) float64        { return a.X*b.X + a.Y*b.Y + a.Z*b.Z }
func Cross(a, b Vec3) Vec3 {
	return Vec3{a.Y*b.Z - a.Z*b.Y, a.Z*b.X - a.X*b.Z, a.X*b.Y - a.Y*b.X}
}
func Len(a Vec3) float64 { return math.Sqrt(Dot(a, a)) }
func Normalize(a Vec3) Vec3 {
	l := Len(a)
	if l == 0 {
		return Vec3{}
	}
	return Scale(a, 1/l)
}

func IdentityQuat() Quat { return Quat{W: 1} }

func QuatFromAxisAngle(axis Vec3, angle float64) Quat {
	a := Normalize(axis)
	h := angle / 2
	s := math.Sin(h)
	return Quat{W: math.Cos(h), X: a.X * s, Y: a.Y * s, Z: a.Z * s}.Normalize()
}

func QuatMul(a, b Quat) Quat {
	return Quat{
		W: a.W*b.W - a.X*b.X - a.Y*b.Y - a.Z*b.Z,
		X: a.W*b.X + a.X*b.W + a.Y*b.Z - a.Z*b.Y,
		Y: a.W*b.Y - a.X*b.Z + a.Y*b.W + a.Z*b.X,
		Z: a.W*b.Z + a.X*b.Y - a.Y*b.X + a.Z*b.W,
	}
}

func (q Quat) Normalize() Quat {
	l := math.Sqrt(q.W*q.W + q.X*q.X + q.Y*q.Y + q.Z*q.Z)
	if l == 0 {
		return IdentityQuat()
	}
	return Quat{q.W / l, q.X / l, q.Y / l, q.Z / l}
}

func (q Quat) Conjugate() Quat { return Quat{W: q.W, X: -q.X, Y: -q.Y, Z: -q.Z} }

func (q Quat) Rotate(v Vec3) Vec3 {
	qv := Quat{W: 0, X: v.X, Y: v.Y, Z: v.Z}
	r := QuatMul(QuatMul(q, qv), q.Conjugate())
	return Vec3{r.X, r.Y, r.Z}
}

// Compose 先应用 child 再应用 parent（parent * child）。
func Compose(parent, child Pose) Pose {
	return Pose{
		Position:    Add(parent.Position, parent.Orientation.Rotate(child.Position)),
		Orientation: QuatMul(parent.Orientation, child.Orientation).Normalize(),
	}
}

// RayRectIntersect 计算射线与以 center 为中心、法线 normal、
// 半尺寸 half 的矩形的交点，返回沿射线的参数 t 与是否命中。
// 约定 right = normal × up(0,1,0) 退化时取 (1,0,0)。
func RayRectIntersect(ray Ray, center Vec3, normal Vec3, half Vec2) (float64, bool) {
	n := Normalize(normal)
	d := Dot(n, ray.Direction)
	const eps = 1e-9
	if math.Abs(d) < eps {
		return 0, false // 射线与面板平面平行
	}
	t := Dot(n, Sub(center, ray.Origin)) / d
	if t < 0 {
		return 0, false // 命中点在射线反方向
	}
	p := Add(ray.Origin, Scale(ray.Direction, t))
	rel := Sub(p, center)
	up := Vec3{0, 1, 0}
	right := Cross(n, up)
	if Len(right) < eps {
		right = Vec3{1, 0, 0}
	}
	right = Normalize(right)
	localUp := Normalize(Cross(right, n))
	u := Dot(rel, right)
	v := Dot(rel, localUp)
	if math.Abs(u) <= half.X && math.Abs(v) <= half.Y {
		return t, true
	}
	return 0, false
}

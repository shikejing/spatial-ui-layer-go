package spatialui

import "math"

// Vec3 是三维向量/点。
type Vec3 struct {
	X, Y, Z float64
}

func (v Vec3) Add(o Vec3) Vec3      { return Vec3{v.X + o.X, v.Y + o.Y, v.Z + o.Z} }
func (v Vec3) Sub(o Vec3) Vec3      { return Vec3{v.X - o.X, v.Y - o.Y, v.Z - o.Z} }
func (v Vec3) Scale(s float64) Vec3 { return Vec3{v.X * s, v.Y * s, v.Z * s} }
func (v Vec3) Dot(o Vec3) float64   { return v.X*o.X + v.Y*o.Y + v.Z*o.Z }
func (v Vec3) Cross(o Vec3) Vec3 {
	return Vec3{v.Y*o.Z - v.Z*o.Y, v.Z*o.X - v.X*o.Z, v.X*o.Y - v.Y*o.X}
}
func (v Vec3) Len() float64 { return math.Sqrt(v.Dot(v)) }
func (v Vec3) Norm() Vec3 {
	l := v.Len()
	if l < 1e-12 {
		return Vec3{}
	}
	return v.Scale(1 / l)
}

// Quat 是单位四元数，表示朝向。
type Quat struct {
	X, Y, Z, W float64
}

func QuatIdentity() Quat { return Quat{W: 1} }

// QuatFromAxisAngle 由单位轴与弧度角构造四元数。
func QuatFromAxisAngle(axis Vec3, rad float64) Quat {
	a := axis.Norm()
	h := rad / 2
	s := math.Sin(h)
	return Quat{X: a.X * s, Y: a.Y * s, Z: a.Z * s, W: math.Cos(h)}.normalized()
}

func (q Quat) normalized() Quat {
	l := math.Sqrt(q.X*q.X + q.Y*q.Y + q.Z*q.Z + q.W*q.W)
	if l < 1e-12 {
		return QuatIdentity()
	}
	return Quat{q.X / l, q.Y / l, q.Z / l, q.W / l}
}

// Mul 组合两个旋转：先应用 o，再应用 q。
func (q Quat) Mul(o Quat) Quat {
	return Quat{
		X: q.W*o.X + q.X*o.W + q.Y*o.Z - q.Z*o.Y,
		Y: q.W*o.Y - q.X*o.Z + q.Y*o.W + q.Z*o.X,
		Z: q.W*o.Z + q.X*o.Y - q.Y*o.X + q.Z*o.W,
		W: q.W*o.W - q.X*o.X - q.Y*o.Y - q.Z*o.Z,
	}
}

func (q Quat) Conj() Quat { return Quat{-q.X, -q.Y, -q.Z, q.W} }

// Rotate 用四元数旋转向量。
func (q Quat) Rotate(v Vec3) Vec3 {
	qv := Vec3{q.X, q.Y, q.Z}
	t := qv.Cross(v).Scale(2)
	return v.Add(t.Scale(q.W)).Add(qv.Cross(t))
}

// Pose 是位置 + 朝向的刚体变换。
type Pose struct {
	Position    Vec3
	Orientation Quat
}

func IdentityPose() Pose { return Pose{Orientation: QuatIdentity()} }

// TransformPoint 将局部点变换到父空间。
func (p Pose) TransformPoint(v Vec3) Vec3 { return p.Orientation.Rotate(v).Add(p.Position) }

// TransformDir 将局部方向变换到父空间（不含平移）。
func (p Pose) TransformDir(v Vec3) Vec3 { return p.Orientation.Rotate(v) }

// Mul 组合位姿：结果等价于先应用 o 再应用 p（p 为父变换）。
func (p Pose) Mul(o Pose) Pose {
	return Pose{
		Position:    p.TransformPoint(o.Position),
		Orientation: p.Orientation.Mul(o.Orientation).normalized(),
	}
}

// Ray 是原点 + 单位方向的射线。
type Ray struct {
	Origin Vec3
	Dir    Vec3
}

// At 返回射线上距离 t 处的点。
func (r Ray) At(t float64) Vec3 { return r.Origin.Add(r.Dir.Scale(t)) }

// 内部三角函数包装，便于测试替换与文档查阅。
func sin(x float64) float64 { return math.Sin(x) }
func cos(x float64) float64 { return math.Cos(x) }

// Inverse 返回刚体变换的逆。
func (p Pose) Inverse() Pose {
	q := p.Orientation.Conj()
	return Pose{Position: q.Rotate(p.Position).Scale(-1), Orientation: q}
}

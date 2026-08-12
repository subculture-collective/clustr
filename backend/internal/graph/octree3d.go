package graph

import "math"

const (
	octree3DMaxDepth = 32
	octree3DMinCell  = 1e-9
)

type pointMass3D struct {
	index   int
	x, y, z float64
	mass    float64
}

type octree3DNode struct {
	cx, cy, cz       float64
	half             float64
	mass             float64
	comX, comY, comZ float64
	points           []pointMass3D
	children         [8]*octree3DNode
}

func (n *octree3DNode) isLeaf() bool { return n.children[0] == nil }
func (n *octree3DNode) contains(x, y, z float64) bool {
	return x >= n.cx-n.half && x <= n.cx+n.half && y >= n.cy-n.half && y <= n.cy+n.half && z >= n.cz-n.half && z <= n.cz+n.half
}

func (n *octree3DNode) insert(point pointMass3D, depth int) {
	if n.mass == 0 {
		n.mass, n.comX, n.comY, n.comZ = point.mass, point.x, point.y, point.z
		n.points = []pointMass3D{point}
		return
	}
	if n.isLeaf() && (depth >= octree3DMaxDepth || n.half*2 <= octree3DMinCell || coincident3D(n.points[0], point)) {
		n.updateMass(point)
		n.points = append(n.points, point)
		return
	}
	if n.isLeaf() {
		old := n.points
		n.points = nil
		n.subdivide()
		for _, existing := range old {
			n.child(existing).insert(existing, depth+1)
		}
	}
	n.updateMass(point)
	n.child(point).insert(point, depth+1)
}

func coincident3D(a, b pointMass3D) bool {
	return math.Abs(a.x-b.x) <= octree3DMinCell && math.Abs(a.y-b.y) <= octree3DMinCell && math.Abs(a.z-b.z) <= octree3DMinCell
}

func (n *octree3DNode) updateMass(point pointMass3D) {
	total := n.mass + point.mass
	n.comX = (n.comX*n.mass + point.x*point.mass) / total
	n.comY = (n.comY*n.mass + point.y*point.mass) / total
	n.comZ = (n.comZ*n.mass + point.z*point.mass) / total
	n.mass = total
}

func (n *octree3DNode) subdivide() {
	quarter := n.half / 2
	for index := 0; index < 8; index++ {
		dx, dy, dz := -quarter, -quarter, -quarter
		if index&1 != 0 {
			dx = quarter
		}
		if index&2 != 0 {
			dy = quarter
		}
		if index&4 != 0 {
			dz = quarter
		}
		n.children[index] = &octree3DNode{cx: n.cx + dx, cy: n.cy + dy, cz: n.cz + dz, half: quarter}
	}
}

func (n *octree3DNode) child(point pointMass3D) *octree3DNode {
	index := 0
	if point.x >= n.cx {
		index |= 1
	}
	if point.y >= n.cy {
		index |= 2
	}
	if point.z >= n.cz {
		index |= 4
	}
	return n.children[index]
}

func buildOctree3D(x, y, z []float64) *octree3DNode {
	if len(x) == 0 || len(x) != len(y) || len(x) != len(z) {
		return nil
	}
	minX, maxX, minY, maxY, minZ, maxZ := x[0], x[0], y[0], y[0], z[0], z[0]
	for i := 1; i < len(x); i++ {
		minX, maxX = math.Min(minX, x[i]), math.Max(maxX, x[i])
		minY, maxY = math.Min(minY, y[i]), math.Max(maxY, y[i])
		minZ, maxZ = math.Min(minZ, z[i]), math.Max(maxZ, z[i])
	}
	size := math.Max(maxX-minX, math.Max(maxY-minY, maxZ-minZ))
	if size < 1 {
		size = 1
	}
	root := &octree3DNode{cx: (minX + maxX) / 2, cy: (minY + maxY) / 2, cz: (minZ + maxZ) / 2, half: size*0.6 + 1e-6}
	for i := range x {
		root.insert(pointMass3D{index: i, x: x[i], y: y[i], z: z[i], mass: 1}, 0)
	}
	return root
}

func (n *octree3DNode) force(index int, x, y, z, theta, strength float64) (float64, float64, float64) {
	if n == nil || n.mass == 0 {
		return 0, 0, 0
	}
	if n.isLeaf() {
		fx, fy, fz := 0.0, 0.0, 0.0
		for _, other := range n.points {
			if other.index == index {
				continue
			}
			dx, dy, dz := other.x-x, other.y-y, other.z-z
			distance := math.Sqrt(dx*dx + dy*dy + dz*dz)
			if distance < 1e-6 {
				dx, dy, dz = deterministicDirection3D(index, other.index)
				distance = 1e-6
			}
			magnitude := strength * other.mass / (distance * distance)
			fx -= dx / distance * magnitude
			fy -= dy / distance * magnitude
			fz -= dz / distance * magnitude
		}
		return fx, fy, fz
	}
	dx, dy, dz := n.comX-x, n.comY-y, n.comZ-z
	distance := math.Sqrt(dx*dx + dy*dy + dz*dz)
	if !n.contains(x, y, z) && distance > 1e-9 && n.half*2/distance < theta {
		magnitude := strength * n.mass / (distance * distance)
		return -dx / distance * magnitude, -dy / distance * magnitude, -dz / distance * magnitude
	}
	fx, fy, fz := 0.0, 0.0, 0.0
	for _, child := range n.children {
		cx, cy, cz := child.force(index, x, y, z, theta, strength)
		fx, fy, fz = fx+cx, fy+cy, fz+cz
	}
	return fx, fy, fz
}

func deterministicDirection3D(a, b int) (float64, float64, float64) {
	h := uint64(uint32(a+1))*0x9e3779b185ebca87 ^ uint64(uint32(b+1))*0xc2b2ae3d27d4eb4f
	u := float64(h&0xffff)/65535*2 - 1
	angle := float64((h>>16)&0xffff) / 65535 * 2 * math.Pi
	radius := math.Sqrt(math.Max(0, 1-u*u))
	return radius * math.Cos(angle) * 1e-6, radius * math.Sin(angle) * 1e-6, u * 1e-6
}

func calculateOctree3DForces(x, y, z, outX, outY, outZ []float64, theta, strength float64) {
	for i := range outX {
		outX[i], outY[i], outZ[i] = 0, 0, 0
	}
	tree := buildOctree3D(x, y, z)
	if tree == nil {
		return
	}
	for i := range x {
		outX[i], outY[i], outZ[i] = tree.force(i, x[i], y[i], z[i], theta, strength)
	}
}

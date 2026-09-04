// vectors.xs — vector literals, members, constructors
vector origin = vector(0, 0, 0);

void place(vector pos) {
	vector offset = (1.0, 2.0, 3.0);
	float dx = pos.x - origin.x;
	origin = offset;
}

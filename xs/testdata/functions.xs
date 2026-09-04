// functions.xs — declarations, params, locals, calls
int add(int a, int b) {
	return a + b;
}

void main() {
	int total = add(1, 2);
	string name = "relic";
	total += 3;
	xsSetRiverHeight(3.0);
}

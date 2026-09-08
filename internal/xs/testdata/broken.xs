// broken.xs — malformed input; the parser must recover, not panic
int ok = 1;

void broken( {
}

void fine() {
	1 + ;
	;
}

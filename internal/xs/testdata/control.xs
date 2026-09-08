// control.xs — switch/for/while/do and branches
void main() {
	for (int i = 0; i < 10; i++) {
		if (i % 2 == 0) {
			continue;
		} else {
			break;
		}
	}

	int n = 0;
	while (n < 3) {
		n++;
	}

	do {
		n--;
	} while (n > 0);

	switch (n) {
	case 0:
		n = 1;
		break;
	case 1:
	case 2:
		n = 2;
		break;
	default:
		n = 0;
	}

	return;
}

// leading comment
int g = 0;

/* block before a declaration */
void work(int n) {
// inside the body
if (n > 0) {
g = g + n;
} else {
g = 0;
}
while (g < 10) {
g = g + 1;
if (g == 5) {
break;
}
}
for (int i = 0; i < n; i = i + 1) {
if (i % 2 == 0) {
continue;
}
}
for (int /* mid-header */ k = 0; k < n; k = k + 1) {
g = g + k;
}
for (int // lifted from the header
	m = 0; m < n; m = m + 1) {
g = g + m;
}
int // lifted from the decl
	quiet = 3;
/* trailing block inside */
return;
}

int after() {
return g; // side comment
}
// tail line comment
/* unterminated block to EOF

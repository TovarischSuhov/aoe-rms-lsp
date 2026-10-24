// c48-lead
vector c48_top = (7, 8);
extern int c48_ext = (9, 10);
int c48_first = 1, c48_second = (11, 12);
int c48_third = (13, 14), c48_fourth = 15;
vector c48_gap = /* c48-in-init */ (20, 21);

void vectors() {
/* c48-before-vec */
(1, 2);
( /* c48-in-vec */ 5, 6);
int v = (3, 4);
}

rule /* c48-mid-rule */ guarded inactive {
// c48-in-rule
xsChat("go");
}

void forComments() {
int total = 0;
for (total = 0; /* c48-in-for */ total < 5; total++) {
total = total + 1;
}
int bare = { int q = 9; // c48-in-bare
q; }; /* c48-after-bare */
}

void c48_switches(int n) {
switch ((16, 17)) {
case 1:
xsChat("one");
default:
xsChat("rest");
}
switch (n) {
case (18, 19):
xsChat("vec");
}
}

void c48_openstr() {
xsChat("start");
"c48-open
}

vector c48_str = "c48-open-decl

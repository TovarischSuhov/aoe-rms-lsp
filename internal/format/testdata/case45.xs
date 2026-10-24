// c45-lead
int total = 0;

void decide(int n) {
switch (n % 3) {
case 0:
total = total + 1;
break;
case /* c45-mid-case */ 1:
total = total + 2;
break;
default:
total = total + 3;
}
}

void loops(int size) {
int i;
for (i = 1; <= size) {
total = total + i;
}
for (int j = 0; j < size; j++) {
total = total + j; // c45-in-for-body
}
}

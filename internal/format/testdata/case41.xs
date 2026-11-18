string tag = "main";
vector home = (1, 2, 3);
int table = 5;

int zoo(int x, int y) {
int r = x + y * 2 - 3;
r = (x + y) * 2;
r = -x + !y + ~r;
r = x << 2 | y & 3;
r = x == y && x != 0 || y < 2;
r = 0xFF & table;
x++;
table[x] = x + f(1, 2, x * 3);
y = home.x;
home.spot(1, 2);
table[0](1);
get(1)(2);
home.spot();
table[0]();
get(1)();
(1, 2, 3)();
r = (1 + 2) * (3 - 4) / 5 % 2;
return r;
}

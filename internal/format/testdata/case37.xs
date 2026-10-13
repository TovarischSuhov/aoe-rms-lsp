// default parameters and extern declarations
int build(int a = -1, /* mid-list */ int size = 0) {
int r = a * size + 1;
return r;
}

float scale( // the note lifts above the head
	float f = 1.5) {
return f * 2.0;
}

extern int goldCount = 0;
extern void onEvent(string name);

int a, b;
int counter = 0, limit = 10;
float ratio = 0.5;

void setup() {
int local = 5;
const int fixed = 7;
float x = 1.0, y = 2.0;
}

rule disable_flashes inactive min-interval 60 // the note lifts above the head
{
set_setting("flashes", 0);
}

event(player_resigned) {
int done = 1;
}

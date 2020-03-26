#include <windows.h>
#include <cstdlib>
#include <iostream.h>
#include <time.h>

#define MAX_WIDTH   25
#define MAX_HEIGHT  50

int width  = 0;
int height = 0;

int delay  = 0;
int fps    = 0;

HANDLE console_handle;
COORD  console_origin = {0, 0};

bool maze[(MAX_HEIGHT + 1)*2 + 1][(MAX_WIDTH + 1)*2 + 1];

int direction_table[4][2];

char output_lookup[] = {(char) ' ', (char) 179, (char) 196, (char) 192,
                        (char) 179, (char) 179, (char) 218, (char) 195,
                        (char) 196, (char) 217, (char) 196, (char) 193,
                        (char) 191, (char) 180, (char) 194, (char) 197};

char big_block[]     = {(char) 219, (char) 219, 0};


void initialize_maze()
{
        int i, j;

        for (i = 1; i < 2*(height+1); i++)
                for (j = 1; j < 2*(width+1); j++)
                        maze[i][j] = 1;

        for (i = 0; i < 2*(height+1) + 1; i++)
                maze[i][0] = maze[i][2*(width+1)] = 0;

        for (j = 0; j < 2*(width+1) + 1; j++)
                maze[0][j] = maze[2*(height+1)][j] = 0;
}


void print_block_maze()
{
        int i, j;

        SetConsoleCursorPosition(console_handle, console_origin);
        for (i = 1; i < 2*(height+1); i++) {
                for (j = 1; j < 2*(width+1); j++) {
                        cout << (maze[i][j] ? big_block : "  ");
                }
                cout << endl;
        }
        cout << endl;
}


void print_line_maze()
{
        int i, j;

        SetConsoleCursorPosition(console_handle, console_origin);
        for (i = 1; i < 2*(height+1); i++) {
                for (j = 1; j < 2*(width+1); j++) {
                        if (maze[i][j]) {
                                cout << output_lookup[maze[i-1][j] + 2*maze[i][j+1] + 4*maze[i+1][j] + 8*maze[i][j-1]];
                                if (!(j & 1))
                                        cout << output_lookup[maze[i-1][j] + 2*maze[i][j+1] + 4*maze[i+1][j] + 8*maze[i][j-1]];
                        }
                        else {
                                cout << " ";
                                if (!(j & 1))
                                        cout << " ";
                        }
                }
                cout << endl;
        }
        cout << endl;
        for (i = 0; i < delay; i++)
                j = j*i;
}


int find_directions(int x, int y)
{
        int possible_directions = 0;

        if (maze[x-2][y]) {             /* look left */
                direction_table[possible_directions][0] = -2;
                direction_table[possible_directions][1] = 0;
                possible_directions++;
        }

        if (maze[x+2][y]) {             /* look right */
                direction_table[possible_directions][0] = 2;
                direction_table[possible_directions][1] = 0;
                possible_directions++;
        }

        if (maze[x][y-2]) {             /* look up */
                direction_table[possible_directions][0] = 0;
                direction_table[possible_directions][1] = -2;
                possible_directions++;
        }

        if (maze[x][y+2]) {             /* look down */
                direction_table[possible_directions][0] = 0;
                direction_table[possible_directions][1] = 2;
                possible_directions++;
        }

        return(possible_directions);
}


int find_path_start(int *path_x, int *path_y)
{
        int i, x_start;
        int j, y_start;

        i = x_start = rand() % height;
        j = y_start = rand() % width;

        for (i = ++i % height; i != x_start; i = ++i % height)
                for (j = ++j % width; j != y_start; j = ++j % width) {
                        *path_x = 2*(i + 1);
                        *path_y = 2*(j + 1);
                        if (maze[*path_x][*path_y] == 0 && find_directions(*path_x, *path_y) != 0)
                                return(1);
                }
        return (0);
}


/*int find_path_start(int *i, int *j)
{
        for (*i = 2; *i <= 2*height; *i += 2)
                for (*j = 2; *j < 2*width; *j += 2)
                        if (find_directions(*i, *j))
                                return(1);
        return(0);
}*/


void carve_path(int x, int y)
{
        int possible_directions;
        int direction;

        maze[x][y] = 0;
        print_line_maze();
        while (possible_directions = find_directions(x, y)) {
                direction = rand() % possible_directions;
                maze[x +  direction_table[direction][0]/2][y +  direction_table[direction][1]/2] = 0;
                print_line_maze();
                maze[x += direction_table[direction][0]  ][y += direction_table[direction][1]  ] = 0;
                print_line_maze();
        }
}


void create_openings()
{
        int start, finish;

        start  = rand() % width;
        finish = rand() % width;

        maze[1]           [2*(start  + 1)] = 0;
        maze[2*height + 1][2*(finish + 1)] = 0;
}


void main()
{
        int path_start_x;
        int path_start_y;

        srand(time(NULL));
        console_handle = GetStdHandle(STD_OUTPUT_HANDLE);

        while (width <= 0 || width > MAX_WIDTH) {
                cout << "input width:  ";
                cin  >> width;
                if (width <= 0 || width > MAX_WIDTH)
                        cout << "invalid width, try again" << endl << endl;
        }
        while (height <= 0 || height > MAX_HEIGHT) {
                cout << "input height: ";
                cin  >> height;
                if (height <= 0 || height > MAX_HEIGHT)
                        cout << "invalid height, try again" << endl << endl;
        }
        while (fps <= 0) {
                cout << "input fps:    ";
                cin  >> fps;
                if (fps <= 0)
                        cout << "invalid fps, try again" << endl << endl;
        }
        delay = 150000000 / fps;

        initialize_maze();

        carve_path(2*((rand() % height) + 1), 2*((rand() % width)  + 1));

        while (find_path_start(&path_start_x, &path_start_y)) {
                carve_path(path_start_x, path_start_y);
        }
        create_openings();

        print_line_maze();
}

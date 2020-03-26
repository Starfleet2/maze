/* maze.c - Simple maze generator
 * By Dirk Gates <dirk.gates@icancelli.com>
 * Copyright 2016 Dirk Gates
 *
 * Revision history
 * ----------------
 * Rev 1.0 -- initial version
 * Rev 1.1 -- add solver
 * Rev 1.2 -- use solver to find best openings top & bottom
 * Rev 1.3 -- only start new paths at the corners of old paths
 * Rev 1.4 -- push mid wall openings left and down when done
 * Rev 1.5 -- add recursive, variable depth look ahead
 * Rev 1.6 -- eliminate 1x1 orphans during construction
 */
#include <stdio.h>
#include <stdlib.h>
#include <unistd.h>
#include <getopt.h>
#include <termios.h>
#include <time.h>
#include <math.h>
#include <sys/time.h>

#define VERSION             "1.6"
#define UTS_SIGN_ON         "\n""Maze Generation Console Utility " VERSION \
                            "\n""Copyright (c) 2016""\n\n"

#define MAX_WIDTH           300
#define MAX_HEIGHT          100
#define MAX_X               ((MAX_HEIGHT + 1)*2 + 1)
#define MAX_Y               ((MAX_WIDTH +  1)*2 + 1)

#define PATH                0
#define WALL                1
#define SOLVED              2
#define TRIED               3
#define CHECK               4

#define UP                  1
#define DOWN                2
#define LEFT                3
#define RIGHT               4

#define NO_SEARCH           0
#define SEARCH              1

#define VT100_LINE_DRAWING
#ifdef  VT100_LINE_DRAWING
#define BLANK               ' '
#define BLOCK               0x61
#define RIGHT_BOTTOM        0x6a
#define RIGHT_TOP           0x6b
#define LEFT_TOP            0x6c
#define LEFT_BOTTOM         0x6d
#define INTERSECTION        0x6e
#define HORIZONTAL          0x71
#define RIGHT_TEE           0x74
#define LEFT_TEE            0x75
#define UP_TEE              0x76
#define DOWN_TEE            0x77
#define VERTICAL            0x78
#else
#define BLANK               ' '
#define BLOCK               '#'
#define RIGHT_BOTTOM        '+'
#define RIGHT_TOP           '+'
#define LEFT_TOP            '+'
#define LEFT_BOTTOM         '+'
#define INTERSECTION        '+'
#define HORIZONTAL          '-'
#define RIGHT_TEE           '+'
#define LEFT_TEE            '+'
#define UP_TEE              '+'
#define DOWN_TEE            '+'
#define VERTICAL            '|'
#endif

char blank_line   [] = "                                                  ";
char output_lookup[] = { BLANK     , VERTICAL    , HORIZONTAL, LEFT_BOTTOM ,
                         VERTICAL  , VERTICAL    , LEFT_TOP  , RIGHT_TEE   ,
                         HORIZONTAL, RIGHT_BOTTOM, HORIZONTAL, UP_TEE      ,
                         RIGHT_TOP , LEFT_TEE    , DOWN_TEE  , INTERSECTION };

char maze[MAX_X][MAX_Y] = {};
struct dir_tbl_type {
    int x;
    int y;
    int heading;
} dir_tbl[4];

int max_x    = 0;
int max_y    = 0;
int width    = 0;
int height   = 0;
int delay    = 0;
int fps      = 0;
int blank    = 0;
int path_len = 0;
int maze_len = 0;
int turn_cnt = 0;
int solves   = 0;
int depth    = 0;
int seed     = 0;
int beg_x, end_x;
int beg_y, end_y;

int path_depth       = 0;
int num_checks       = 0;
int max_checks       = 0;
int num_paths        = 0;
int num_solves       = 0;
int num_wall_push    = 0;
int max_path_length  = 0;
int num_maze_created = 0;

#define ms_sleep(n)         ({ int _n = (n); struct timespec tr = {_n / 1000, (_n % 1000) * 1000 * 1000}; nanosleep(&tr, NULL); })   // millisecond sleep

#define min(x,y)            ({ typeof(x) _x = (x), _y = (y); (_x < _y) ? _x : _y; })
#define max(x,y)            ({ typeof(x) _x = (x), _y = (y); (_x > _y) ? _x : _y; })

#define is_even(x)          (((x) & 1) == 0)
#define is_odd(x)           (((x) & 1) == 1)

#define set_position(x, y)  printf("\033[%d;%dH", x, y)
#define set_line_draw()     printf("\033(0")
#define clr_line_draw()     printf("\033(B")
#define set_cursor_off()    printf("\033[?25l")
#define set_cursor_on()     printf("\033[?25h")
#define clr_screen()        printf("\033[2J")
#define set_solved()        printf("\033[32m\033[1m")
#define clr_solved()        printf("\033[30m\033[0m")


void get_console_size(int *rows, int *cols)
{
    struct termios org_terminal;
    struct termios raw_terminal;

    tcgetattr(STDIN_FILENO, &org_terminal);
    tcgetattr(STDIN_FILENO, &raw_terminal);
    raw_terminal.c_lflag &= ~(ECHO | ICANON);       // do not echo input characters; input available character by character
    tcsetattr(STDIN_FILENO, TCSANOW, &raw_terminal);

    printf("\0337");                                // save the current cursor position
    printf("\033[999;999H");                        // put the cursor way out there
    printf("\033[6n");                              // request the current cursor position
    if (scanf("\033[%d;%dR", rows, cols) != 2) {    // and find out where it ended up
        *cols = 80; *rows = 24;                     // (and if we can't, just
    }                                               //  assume 80x24, oh well ...)
    printf("\0338");                                // restore the original cursor position

    tcsetattr(STDIN_FILENO, TCSANOW, &org_terminal);
}


void initialize_maze(int *x, int *y)
{
    int i, j;

    max_x = 2*(height + 1) + 1;
    max_y = 2*(width  + 1) + 1;

    for (i = 1; i < max_x - 1; i++) {
        for (j = 1; j < max_y - 1; j++) {
            maze[i][j] = WALL;
        }
    }
    for (i = 0; i < max_x; i++) maze[i][0] = maze[i][2*(width  + 1)] = PATH;
    for (j = 0; j < max_y; j++) maze[0][j] = maze[2*(height + 1)][j] = PATH;

    *x = 2*((rand() % height) + 1); // random location
    *y = 2*((rand() % width ) + 1); // for first path

    beg_x =  2;                     // these will
    end_x =  2*height;              // never change
}


void restore_maze(void)
{
    int i, j;

    for (i = 0; i < max_x; i++) {
        for (j = 0; j < max_y; j++) {
            if (maze[i][j] == SOLVED ||
                maze[i][j] == TRIED)
                maze[i][j] =  PATH;
        }
    }
}


void print_maze()
{
    int i, j;

    set_position(0, 0);
    set_line_draw();

    for (i = 1; i < 2 * (height + 1); i++) {                                            // wall intersection point                             // non-intersection point
        for (j = 1; j < 2 * (width + 1); j++) {                                         // check that there is a path on the diagonal          // check that there is a path adjacent
            char v = output_lookup[1*(maze[i-1][j] == WALL && ((is_odd(i) && is_odd(j)) ? (maze[i-1][j-1] != WALL || maze[i-1][j+1] != WALL) : (maze[i  ][j-1] != WALL || maze[i  ][j+1] != WALL))) +
                                   2*(maze[i][j+1] == WALL && ((is_odd(i) && is_odd(j)) ? (maze[i-1][j+1] != WALL || maze[i+1][j+1] != WALL) : (maze[i-1][j  ] != WALL || maze[i+1][j  ] != WALL))) +
                                   4*(maze[i+1][j] == WALL && ((is_odd(i) && is_odd(j)) ? (maze[i+1][j-1] != WALL || maze[i+1][j+1] != WALL) : (maze[i  ][j-1] != WALL || maze[i  ][j+1] != WALL))) +
                                   8*(maze[i][j-1] == WALL && ((is_odd(i) && is_odd(j)) ? (maze[i-1][j-1] != WALL || maze[i+1][j-1] != WALL) : (maze[i-1][j  ] != WALL || maze[i+1][j  ] != WALL)))];
            char s = output_lookup[1*(maze[i-1][j] == maze[i][j]) +
                                   2*(maze[i][j+1] == maze[i][j]) +
                                   4*(maze[i+1][j] == maze[i][j]) +
                                   8*(maze[i][j-1] == maze[i][j])];
            char l = (is_even(i)  &&  maze[i][j-1] == SOLVED) ? HORIZONTAL : BLANK;
            char r = (is_even(i)  &&  maze[i][j+1] == SOLVED) ? HORIZONTAL : BLANK;
            char w = blank ? v : s;

            switch (maze[i][j]) {
                case WALL  :               putchar( w ); if (is_even(j)) { putchar( w ); putchar( w ); }               break;
                case SOLVED: set_solved(); putchar( l ); if (is_even(j)) { putchar( s ); putchar( r ); } clr_solved(); break;
                case CHECK :               putchar(' '); if (is_even(j)) { putchar('#'); putchar(' '); }               break;
                default    :               putchar(' '); if (is_even(j)) { putchar(' '); putchar(' '); }               break;
            }
        }
        putchar('\n');
    }
    clr_line_draw();

    printf("height=%d, width=%d, seed=%d, max_checks=%d, num_wall_push=%d, num_maze_created=%d, num_solves=%d, maze_len=%d, num_paths=%d, avg_path_length=%d, max_path_length=%d %s\r",
            height   , width   , seed   , max_checks   , num_wall_push   , num_maze_created   , num_solves   , maze_len   , num_paths   , maze_len/num_paths, max_path_length, blank_line);

    if (delay)
        ms_sleep(delay);
}

int check_directions(int x, int y, int val, int depth, int *checks)
{
    int ret = 1;

    if (depth && *checks < 500000) {
        ++*checks;
        if (max_checks < ++num_checks)
            max_checks =   num_checks;

                maze[x    ][y] =  CHECK;
        ret = ((maze[x - 1][y] == val && maze[x - 2][y] == val && check_directions(x - 2, y, val, depth - 1, checks)) ||   // look left
               (maze[x + 1][y] == val && maze[x + 2][y] == val && check_directions(x + 2, y, val, depth - 1, checks)) ||   // look right
               (maze[x][y - 1] == val && maze[x][y - 2] == val && check_directions(x, y - 2, val, depth - 1, checks)) ||   // look up
               (maze[x][y + 1] == val && maze[x][y + 2] == val && check_directions(x, y + 2, val, depth - 1, checks)));    // look down
                maze[x][y    ] =  val;
    }
    return (ret);
}

int orphan_1x1(int x, int y)
{
    return (maze[x - 1][y] == WALL && maze[x - 2][y] == PATH &&     // horizontal (look left & right)
            maze[x + 1][y] == WALL && maze[x + 2][y] == PATH &&
            maze[x][y - 1] == WALL && maze[x][y - 2] == PATH &&     // vertical   (look up & down)
            maze[x][y + 1] == WALL && maze[x][y + 2] == PATH);
}

int check_orphan(int x, int y, int dx, int dy, int depth)
{
    int orphan = 0;

    if (depth) {                                        // this only makes sense when carving paths, not when solving, and only if we haven't exhausted our search depth
        maze[x + dx/2][y + dy/2] = PATH;                // temporarily set new path
        maze[x + dx  ][y + dy  ] = PATH;

        orphan = orphan_1x1(x + dx - 2, y + dy    ) ||  // check for 1x1 orphans left & right of the new location
                 orphan_1x1(x + dx + 2, y + dy    ) ||
                 orphan_1x1(x + dx    , y + dy - 2) ||  // check for 1x1 orphans above & below of the new location
                 orphan_1x1(x + dx    , y + dy + 2);

        maze[x + dx  ][y + dy  ] = WALL;                // restore original walls
        maze[x + dx/2][y + dy/2] = WALL;
    }
    return (orphan);
}

int look(int heading, int x, int y, int dx, int dy, int n, int val, int depth)
{
    int check = 0;

    if (maze[x + dx/2][y + dy/2] == val &&
        maze[x + dx  ][y + dy  ] == val && !check_orphan(x, y, dx, dy, depth) && check_directions(x + dx, y + dy, val, depth, &check)) {
        dir_tbl[n].x = dx;
        dir_tbl[n].y = dy;
        dir_tbl[n].heading = heading;
        return (1);
    }
    return (0);
}

int find_directions(int x, int y, int val, int search)
{
    int n = 0;

    num_checks = 0;
    do {
        n += look(LEFT , x, y, -2,  0, n, val, search ? path_depth : 0);
        n += look(RIGHT, x, y,  2,  0, n, val, search ? path_depth : 0);
        n += look(UP   , x, y,  0, -2, n, val, search ? path_depth : 0);
        n += look(DOWN , x, y,  0,  2, n, val, search ? path_depth : 0);
    } while (!n && search && path_depth--);
    if (path_depth < 0) {
        path_depth = 0;
    }
    return (n);
}

int straight_thru(int x, int y, int val)
{
    return ((maze[x - 1][y] == val && maze[x - 2][y] == val &&     // horizontal (look left & right)
             maze[x + 1][y] == val && maze[x + 2][y] == val) ||
            (maze[x][y - 1] == val && maze[x][y - 2] == val &&     // vertical   (look up & down)
             maze[x][y + 1] == val && maze[x][y + 2] == val));
}

int find_path_start(int *x, int *y)
{
    path_depth = depth;
    do {
        int x_start = rand() % height;
        int y_start = rand() % width ;
        int i, j;

        for (i = 0; i < height; ++i) {
            for (j = 0; j < width; ++j) {
                *x = 2*((x_start + i) % height + 1);
                *y = 2*((y_start + j) % width  + 1);
                if (maze[*x][*y] == PATH && !straight_thru(*x, *y, PATH) && find_directions(*x, *y, WALL, NO_SEARCH)) {
                    return (1);
                }
            }
        }
    } while (path_depth--);
    if (path_depth < 0) {
        path_depth = 0;
    }
    return (0);
}

void mark_cell(int x, int y, int val)
{
    if (maze[x][y] != val) {
        maze[x][y]  = val;
        if (delay && fps <= 1000 && is_even(x) && is_even(y))
            print_maze();
    }
}

void carve_path(int *x, int *y)
{
    int n, dir;

    path_depth = depth;
    mark_cell(*x, *y, PATH);
    while ((n = find_directions(*x, *y, WALL, SEARCH)) != 0) {
        dir = rand() % n;
        mark_cell(*x +  dir_tbl[dir].x/2, *y +  dir_tbl[dir].y/2, PATH);
        mark_cell(*x += dir_tbl[dir].x  , *y += dir_tbl[dir].y  , PATH);
        maze_len++;
    }
    if (delay)
        print_maze();
}

int follow_path(int *x, int *y) {
    int last_dir = 0;

    path_depth = 0;
    mark_cell(*x, *y, SOLVED);
    while (beg_x <= *x && *x <= end_x && find_directions(*x, *y, PATH, NO_SEARCH)) {
        mark_cell(*x +  dir_tbl[0].x/2, *y +  dir_tbl[0].y/2, SOLVED);
        mark_cell(*x += dir_tbl[0].x  , *y += dir_tbl[0].y  , SOLVED);
        path_len++;
        if (last_dir != dir_tbl[0].heading) {
            last_dir  = dir_tbl[0].heading;
            turn_cnt++;
        }
    }
    return (*x > end_x);
}

void back_track_path(int *x, int *y) {
    int last_dir = 0;

    path_depth = 0;
    mark_cell(*x, *y, TRIED);
    while (!find_directions(*x, *y, PATH, NO_SEARCH) && find_directions(*x, *y, SOLVED, NO_SEARCH)) {
        mark_cell(*x +  dir_tbl[0].x/2, *y +  dir_tbl[0].y/2, TRIED);
        mark_cell(*x += dir_tbl[0].x  , *y += dir_tbl[0].y  , TRIED);
        path_len--;
        if (last_dir != dir_tbl[0].heading) {
            last_dir  = dir_tbl[0].heading;
            turn_cnt--;
        }
    }
}

void solve_maze(int *x, int *y)
{
    path_len = 0;
    turn_cnt = 0;

    maze[beg_x - 1][beg_y] = SOLVED;
    while (!follow_path(x, y)) {
        back_track_path(x, y);
    }
    maze[end_x + 1][end_y] = SOLVED;
    solves++;
}

void create_openings(int *x, int *y)
{
    beg_y = *x;
    end_y = *y;

    maze[beg_x - 1][beg_y] = PATH;
    maze[end_x + 1][end_y] = PATH;

    *x = beg_x;
    *y = beg_y;
}

void delete_openings(void)
{
    maze[beg_x - 1][beg_y] = WALL;
    maze[end_x + 1][end_y] = WALL;
}

void search_best_openings(int *x, int *y)
{
    int best_path_len = 0;
    int best_turn_cnt = 0;
    int best_start    = 2;
    int best_finish   = 2;
    int start ;
    int finish;
    int i, j;

    for (i = 0; i < width; ++i) {
        for (j = 0; j < width; ++j) {
            start  = *x = 2*(i + 1);
            finish = *y = 2*(j + 1);
            if (maze[beg_x][start  - 1] != WALL && maze[beg_x][start  + 1] != WALL) continue;
            if (maze[end_x][finish - 1] != WALL && maze[end_x][finish + 1] != WALL) continue;
            create_openings(x, y);
            solve_maze(x, y);
            if (path_len >  best_path_len ||
               (path_len == best_path_len &&
                turn_cnt >  best_turn_cnt)) {
                best_start      = start   ;
                best_finish     = finish  ;
                best_turn_cnt   = turn_cnt;
                best_path_len   = path_len;
                max_path_length = path_len;
            }
            restore_maze();
            delete_openings();
            ++num_solves;
        }
    }
    *x = best_start;
    *y = best_finish;
    create_openings(x, y);
}

int mid_wall_opening(int x, int y)
{
    return (maze[x    ][y    ] == PATH &&
            maze[x - 1][y - 1] != WALL &&
            maze[x - 1][y + 1] != WALL &&
            maze[x + 1][y - 1] != WALL &&
            maze[x + 1][y + 1] != WALL);
}

int push_mid_wall_openings()
{
    int moves = 0;
    int i, j;

    for (i = 1; i < 2 * (height + 1); ++i) {
        for (j = (i & 1) + 1; j < 2 * (width + 1); j += 2) {
            if (mid_wall_opening(i, j)) {
                mark_cell(i, j, WALL);
                if is_odd(i) mark_cell(i, j + 2, PATH);    // push right
                else         mark_cell(i + 2, j, PATH);    // push down
                ++moves;
                ++num_wall_push;
            }
        }
    }
    if (delay)
        print_maze();
    return (moves);
}

void create_maze(int *x, int *y)
{
    max_checks = 0;
    maze_len   = 0;
    num_paths  = 0;

    initialize_maze(x, y);
    do {
        num_paths++;
        carve_path(x, y);
    } while (find_path_start(x, y) != 0);

    while (push_mid_wall_openings())
        ;

    delay = 0;  // don't print updates while solving for best openings
    search_best_openings(x, y);
}

int main(int argc, char *argv[])
{
    struct timeval tval;
    struct option  long_opts[] = {
        { "blank" , 0, NULL, 'b' },
        { "depth" , 1, NULL, 'd' },
        { "fps"   , 1, NULL, 'f' },
        { "height", 1, NULL, 'h' },
        { "path"  , 2, NULL, 'p' },
        { "show"  , 0, NULL, 's' },
        { "width" , 1, NULL, 'w' },
        { NULL    , 0, NULL,  0  }
    };
    int rows = 24;
    int cols = 80;
    int opt  =  0;
    int path_start_x;
    int path_start_y;
    int max_height = MAX_HEIGHT;
    int max_width  = MAX_WIDTH;
    int min_path_length = 1;
    int show = 0;

    get_console_size(&rows, &cols);

    height = max_height = min(MAX_HEIGHT, (rows -= 3)/2);
    width  = max_width  = min(MAX_WIDTH , (cols -= 1)/4);

    while ((opt = getopt_long(argc, argv, "bd:f:h:p::r:sw:", long_opts, NULL)) != -1) {
        switch (opt) {
            case 'd': depth  = atoi(optarg); break;
            case 'f': fps    = atoi(optarg); break;
            case 'h': height = atoi(optarg); break;
            case 'w': width  = atoi(optarg); break;
            case 'r': seed   = atoi(optarg); break;
            case 'b': blank  = 1           ; break;
            case 's': show   = 1           ; break;
            case 'p': {
                char *path_arg = optarg;
                if (!optarg && argv[optind] && argv[optind][0] != '-')
                    path_arg = argv[optind++];
                if (path_arg) min_path_length = atoi(path_arg);
                else          min_path_length = 0;
                break;
            }
            default :
                printf("%s\nUsage: %s [options]\n%s", UTS_SIGN_ON, argv[0],
                       "Options:"                                                                                "\n"
                       "  -f, --fps     <frames per second>  Set refresh rate           (default: none, instant)""\n"
                       "  -h, --height  <height>             Set maze height            (default: screen height)""\n"
                       "  -w, --width   <width>              Set maze width             (default: screen width )""\n"
                       "  -d, --depth   <depth>              Set path search depth      (default: 1            )""\n"
                       "  -p, --path   [<length>]            Set minimum path length    (default: none         )""\n"
                       "  -r, --random  <seed>               Set random number seed     (default: current usec )""\n"
                       "  -s, --show                         Show intermediate results while path length not met""\n"
                       "  -b, --blank                        Show empty maze as blank vs. lattice work of walls ""\n\n");
                exit(0);
                break;
        }
    }

    if (depth  <  0 || depth  > 100       ) depth  = 100   ;
    if (fps    <  0 || fps    > 100000    ) fps    = 100000;
    if (height <= 0 || height > max_height) height = max_height;
    if (width  <= 0 || width  > max_width ) width  = max_width ;

    if (min_path_length <  0 || min_path_length >= height * width)
        min_path_length =  0;

    if (min_path_length == 0)
         min_path_length = min((height * width) / 2, (int)sqrt(height * width) * 10);

    clr_screen();
    set_cursor_off();

    do {
        if (fps) {
            delay = ((fps > 1000) ? 1000000 : 1000) / fps;
        }
        if (num_maze_created++ > 0 || !seed) {
            gettimeofday(&tval, NULL);
            seed = tval.tv_usec;
        }
        srand(seed);

        create_maze(&path_start_x, &path_start_y); if (show) { print_maze(); sleep(1); }
         solve_maze(&path_start_x, &path_start_y); if (show) { print_maze(); sleep(1); }

    } while (max_path_length < min_path_length);

    print_maze();
    set_cursor_on();
    printf("\n");
    return (0);
}


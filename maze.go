/* maze.c - Simple maze generator
 * By Dirk Gates <dirk.gates@icancelli.com>
 * Copyright 2016-2020 Dirk Gates
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
 * Rev 2.0 -- translated to go
 */
package main

import (
	"os"
	"fmt"
	"bufio"
	"flag"
	"time"
	"math"
	"math/rand"
	"golang.org/x/crypto/ssh/terminal"
)

const (
	VERSION             = "2.0"
	UTS_SIGN_ON         = "\n" + "Maze Generation Console Utility "+ VERSION +
                          "\n" + "Copyright (c) 2016-2020" +
		                  "\n\n"
    BLANK_LINE          = "                                                  ";

	MAX_WIDTH           = 300
	MAX_HEIGHT          = 100
	MAX_X               = ((MAX_HEIGHT + 1)*2 + 1)
	MAX_Y               = ((MAX_WIDTH +  1)*2 + 1)

	PATH                = 0
	WALL                = 1
	SOLVED              = 2
	TRIED               = 3
	CHECK               = 4

	UP                  = 1
	DOWN                = 2
	LEFT                = 3
	RIGHT               = 4

	NO_SEARCH           = false
	SEARCH              = true

	BLANK               = ' '  // ' '
	BLOCK               = 0x61 // '#'
	RIGHT_BOTTOM        = 0x6a // '+'
	RIGHT_TOP           = 0x6b // '+'
	LEFT_TOP            = 0x6c // '+'
	LEFT_BOTTOM         = 0x6d // '+'
	INTERSECTION        = 0x6e // '+'
	HORIZONTAL          = 0x71 // '-'
	RIGHT_TEE           = 0x74 // '+'
	LEFT_TEE            = 0x75 // '+'
	UP_TEE              = 0x76 // '+'
	DOWN_TEE            = 0x77 // '+'
	VERTICAL            = 0x78 // '|'
)

type dir_tbl_type struct {
    x       int
    y       int
    heading int
}

var (
	output_lookup = [16]byte { BLANK     , VERTICAL    , HORIZONTAL, LEFT_BOTTOM ,
							   VERTICAL  , VERTICAL    , LEFT_TOP  , RIGHT_TEE   ,
							   HORIZONTAL, RIGHT_BOTTOM, HORIZONTAL, UP_TEE      ,
							   RIGHT_TOP , LEFT_TEE    , DOWN_TEE  , INTERSECTION }
    maze[MAX_X][MAX_Y] byte
	dir_tbl[4]		   dir_tbl_type

	blank              bool
    show               bool

	max_x, max_y       int
	beg_x, end_x       int
	beg_y, end_y       int
	width              int
	height             int
	delay              int
	fps                int
	min_len            int
	path_len           int
	maze_len           int
	turn_cnt           int
	solves             int
	depth              int
	seed               int
	path_depth         int
	num_checks         int
	max_checks         int
	num_paths          int
	num_solves         int
	num_wall_push      int
	max_path_length    int
	num_maze_created   int

	my_stdout          *bufio.Writer
)

func ms_sleep(n int)     	{; time.Sleep(time.Duration(int64(n) * 1000 * 1000)); }

func bool2int(b bool) int 	{; if b      {; return 1; }; return 0; }
func min(x, y    int) int   {; if x <  y {; return x; }; return y; }
func max(x, y    int) int   {; if x >  y {; return x; }; return y; }
func non_zero(x  int) int   {; if x != 0 {; return x; }; return 1; }

func is_even(x   int) bool  {; return (x & 1) == 0; }
func is_odd( x   int) bool  {; return (x & 1) != 0; }

func putchar(c byte)        {; my_stdout.WriteByte(c); }

func set_position(x, y int) {; fmt.Fprintf(my_stdout, "\033[%d;%dH", x, y); my_stdout.Flush(); }
func set_line_draw()        {; fmt.Fprintf(my_stdout, "\033(0"           ); my_stdout.Flush(); }
func clr_line_draw()        {; fmt.Fprintf(my_stdout, "\033(B"           ); my_stdout.Flush(); }
func set_cursor_off()       {; fmt.Fprintf(my_stdout, "\033[?25l"        ); my_stdout.Flush(); }
func set_cursor_on()        {; fmt.Fprintf(my_stdout, "\033[?25h"        ); my_stdout.Flush(); }
func clr_screen()           {; fmt.Fprintf(my_stdout, "\033[2J"          ); my_stdout.Flush(); }
func set_solved()           {; fmt.Fprintf(my_stdout, "\033[32m\033[1m"  ); my_stdout.Flush(); }
func clr_solved()           {; fmt.Fprintf(my_stdout, "\033[30m\033[0m"  ); my_stdout.Flush(); }


func get_console_size() (int, int) {
    cols, rows, err := terminal.GetSize(0)
	if err != nil {
		rows = 24
		cols = 80
	}
	return rows, cols
}

func initialize_maze(x, y *int) {
    max_x = 2*(height + 1) + 1
    max_y = 2*(width  + 1) + 1

    for i := 1; i < max_x - 1; i++ {
        for j := 1; j < max_y - 1; j++ {
            maze[i][j] = WALL
        }
    }
    for i := 0; i < max_x; i++ {; maze[i][0] = PATH; maze[i][2*(width  + 1)] = PATH; }
    for j := 0; j < max_y; j++ {; maze[0][j] = PATH; maze[2*(height + 1)][j] = PATH; }

    *x = 2*((rand.Intn(height)) + 1) 	// random location
    *y = 2*((rand.Intn(width )) + 1)	// for first path

    beg_x =  2                       	// these will
    end_x =  2*height                	// never change
}

func restore_maze() {
    for i := 0; i < max_x; i++ {
        for j := 0; j < max_y; j++ {
            if (maze[i][j] == SOLVED ||
                maze[i][j] == TRIED) {
                maze[i][j] =  PATH
			}
		}
    }
}


func print_maze() {
    set_position(0, 0)
    set_line_draw()

    for i := 1; i < 2 * (height + 1); i++ {
        for j := 1; j < 2 * (width + 1); j++ {
			var v, s, l, r, w byte

			if is_odd(i) && is_odd(j) {
				v = output_lookup[1 * bool2int(maze[i-1][j] == WALL && (maze[i-1][j-1] != WALL || maze[i-1][j+1] != WALL)) +	// wall intersection point
								  2 * bool2int(maze[i][j+1] == WALL && (maze[i-1][j+1] != WALL || maze[i+1][j+1] != WALL)) +	// check that there is a path on the diagonal
								  4 * bool2int(maze[i+1][j] == WALL && (maze[i+1][j-1] != WALL || maze[i+1][j+1] != WALL)) +
								  8 * bool2int(maze[i][j-1] == WALL && (maze[i-1][j-1] != WALL || maze[i+1][j-1] != WALL))];
			} else {
				v = output_lookup[1 * bool2int(maze[i-1][j] == WALL && (maze[i  ][j-1] != WALL || maze[i  ][j+1] != WALL)) +	// non-intersection point
								  2 * bool2int(maze[i][j+1] == WALL && (maze[i-1][j  ] != WALL || maze[i+1][j  ] != WALL)) +	// check that there is a path adjacent
								  4 * bool2int(maze[i+1][j] == WALL && (maze[i  ][j-1] != WALL || maze[i  ][j+1] != WALL)) +
								  8 * bool2int(maze[i][j-1] == WALL && (maze[i-1][j  ] != WALL || maze[i+1][j  ] != WALL))];
			}
                s = output_lookup[1 * bool2int(maze[i-1][j] == maze[i][j]) +
                                  2 * bool2int(maze[i][j+1] == maze[i][j]) +
                                  4 * bool2int(maze[i+1][j] == maze[i][j]) +
                                  8 * bool2int(maze[i][j-1] == maze[i][j])]

            if is_even(i) && maze[i][j-1] == SOLVED {; l = HORIZONTAL; } else {; l = BLANK; }
            if is_even(i) && maze[i][j+1] == SOLVED {; r = HORIZONTAL; } else {; r = BLANK; }

			if blank {; w = v; } else {; w = s; }

            switch (maze[i][j]) {
                case WALL  :               putchar( w ); if (is_even(j)) {; putchar( w ); putchar( w ); }
                case SOLVED: set_solved(); putchar( l ); if (is_even(j)) {; putchar( s ); putchar( r ); }; clr_solved()
                case CHECK :               putchar(' '); if (is_even(j)) {; putchar('#'); putchar(' '); }
                default    :               putchar(' '); if (is_even(j)) {; putchar(' '); putchar(' '); }
            }
        }
        putchar('\n')
    }
    clr_line_draw()

    fmt.Fprintf(my_stdout, "height=%d, width=%d, seed=%d, max_checks=%d, num_wall_push=%d, num_maze_created=%d, num_solves=%d, maze_len=%d, num_paths=%d, avg_path_length=%d, " +      "max_path_length=%d %s\r",
                            height   , width   , seed   , max_checks   , num_wall_push   , num_maze_created   , num_solves   , maze_len   , num_paths   , maze_len/non_zero(num_paths), max_path_length, BLANK_LINE);
    if delay != 0 {
    	ms_sleep(delay)
	}
}

func check_directions(x, y int, val byte, depth int, checks *int) bool {
    match := true

    if depth != 0 && *checks < 500000 {
        *checks++
		num_checks++
        if max_checks < num_checks {
           max_checks = num_checks
		}
                 maze[x    ][y] =  CHECK
        match = (maze[x - 1][y] == val && maze[x - 2][y] == val && check_directions(x - 2, y, val, depth - 1, checks)) ||	// look left
                (maze[x + 1][y] == val && maze[x + 2][y] == val && check_directions(x + 2, y, val, depth - 1, checks)) ||   // look right
                (maze[x][y - 1] == val && maze[x][y - 2] == val && check_directions(x, y - 2, val, depth - 1, checks)) ||   // look up
                (maze[x][y + 1] == val && maze[x][y + 2] == val && check_directions(x, y + 2, val, depth - 1, checks))      // look down
                 maze[x][y    ] =  val
    }
    return match
}

func orphan_1x1(x, y int) bool {
    return      x > 1             &&      y > 1             &&  // bounds check
	       maze[x - 1][y] == WALL && maze[x - 2][y] == PATH &&	// horizontal (look left & right)
           maze[x + 1][y] == WALL && maze[x + 2][y] == PATH &&
           maze[x][y - 1] == WALL && maze[x][y - 2] == PATH &&	// vertical   (look up & down)
           maze[x][y + 1] == WALL && maze[x][y + 2] == PATH
}

func check_orphan(x, y, dx, dy, depth int) bool {
    orphan := false;

    if depth != 0 {                                     // this only makes sense when carving paths, not when solving, and only if we haven't exhausted our search depth
        maze[x + dx/2][y + dy/2] = PATH                 // temporarily set new path
        maze[x + dx  ][y + dy  ] = PATH

        orphan = orphan_1x1(x + dx - 2, y + dy    ) ||  // check for 1x1 orphans left & right of the new location
                 orphan_1x1(x + dx + 2, y + dy    ) ||
                 orphan_1x1(x + dx    , y + dy - 2) ||  // check for 1x1 orphans above & below of the new location
                 orphan_1x1(x + dx    , y + dy + 2)

        maze[x + dx  ][y + dy  ] = WALL                 // restore original walls
        maze[x + dx/2][y + dy/2] = WALL
    }
    return orphan
}

func look(heading, x, y, dx, dy, num int, val byte, depth int) int {
    check := 0
    if maze[x + dx/2][y + dy/2] == val &&
       maze[x + dx  ][y + dy  ] == val && !check_orphan(x, y, dx, dy, depth) && check_directions(x + dx, y + dy, val, depth, &check) {
        dir_tbl[num].x = dx
        dir_tbl[num].y = dy
        dir_tbl[num].heading = heading
        return 1
    }
    return 0
}

func find_directions(x, y int, val byte, search bool) int {
    num          := 0
	search_depth := 0
    num_checks    = 0
    for {
		if search {
			search_depth = path_depth
		}
        num += look(LEFT , x, y, -2,  0, num, val, search_depth);
        num += look(RIGHT, x, y,  2,  0, num, val, search_depth);
        num += look(UP   , x, y,  0, -2, num, val, search_depth);
        num += look(DOWN , x, y,  0,  2, num, val, search_depth);

		if num != 0 || search == false || path_depth == 0 {
		   break
		}
		path_depth--
    }
    return (num);
}

func straight_thru(x, y int, val byte) bool {
    return (maze[x - 1][y] == val && maze[x - 2][y] == val  &&	// horizontal (look left & right)
            maze[x + 1][y] == val && maze[x + 2][y] == val) ||
           (maze[x][y - 1] == val && maze[x][y - 2] == val  &&	// vertical   (look up & down)
            maze[x][y + 1] == val && maze[x][y + 2] == val)
}

func find_path_start(x, y *int) bool {
    path_depth = depth;
    for {
        x_start := rand.Intn(height)
        y_start := rand.Intn(width )

        for i := 0; i < height; i++ {
            for j := 0; j < width; j++ {
                *x = 2*((x_start + i) % height + 1)
                *y = 2*((y_start + j) % width  + 1)
                if (maze[*x][*y] == PATH && !straight_thru(*x, *y, PATH) && find_directions(*x, *y, WALL, NO_SEARCH) != 0) {
                    return true
                }
            }
        }
		if path_depth == 0 {
		   break
		}
		path_depth--
    }
    return false
}

func mark_cell(x, y int, val byte) {
    if (maze[x][y] != val) {
        maze[x][y]  = val
        if delay != 0 && fps <= 1000 && is_even(x) && is_even(y) {
            print_maze()
		}
    }
}

func carve_path(x, y *int) {
    path_depth = depth
    mark_cell(*x, *y, PATH)
	for {
		num := find_directions(*x, *y, WALL, SEARCH)
		if num == 0 {
		   break
		}
        dir := rand.Intn(num)
        mark_cell(*x +  dir_tbl[dir].x/2, *y +  dir_tbl[dir].y/2, PATH)
        mark_cell(*x +  dir_tbl[dir].x  , *y +  dir_tbl[dir].y  , PATH)
                  *x += dir_tbl[dir].x  ; *y += dir_tbl[dir].y
		maze_len++
    }
    if delay != 0 {
        print_maze()
	}
}

func follow_path(x, y *int) bool {
    last_dir  := 0
    path_depth = 0
    mark_cell(*x, *y, SOLVED)
    for ; beg_x <= *x && *x <= end_x && find_directions(*x, *y, PATH, NO_SEARCH) != 0 ; {
        mark_cell(*x +  dir_tbl[0].x/2, *y +  dir_tbl[0].y/2, SOLVED)
        mark_cell(*x +  dir_tbl[0].x  , *y +  dir_tbl[0].y  , SOLVED)
                  *x += dir_tbl[0].x  ; *y += dir_tbl[0].y
        path_len++
        if (last_dir != dir_tbl[0].heading) {
            last_dir  = dir_tbl[0].heading
            turn_cnt++
        }
    }
    return *x > end_x
}

func back_track_path(x, y *int) {
    last_dir  := 0
    path_depth = 0
    mark_cell(*x, *y, TRIED)
    for ; find_directions(*x, *y, PATH, NO_SEARCH) == 0 && find_directions(*x, *y, SOLVED, NO_SEARCH) != 0 ; {
        mark_cell(*x +  dir_tbl[0].x/2, *y +  dir_tbl[0].y/2, TRIED)
        mark_cell(*x +  dir_tbl[0].x  , *y +  dir_tbl[0].y  , TRIED)
                  *x += dir_tbl[0].x  ; *y += dir_tbl[0].y
        path_len--
        if (last_dir != dir_tbl[0].heading) {
            last_dir  = dir_tbl[0].heading;
            turn_cnt--
        }
    }
}

func solve_maze(x, y *int) {
    path_len = 0
    turn_cnt = 0

    maze[beg_x - 1][beg_y] = SOLVED
    for ;  !follow_path(x, y) ; {
        back_track_path(x, y)
    }
    maze[end_x + 1][end_y] = SOLVED
    solves++
}

func create_openings(x, y *int) {
    beg_y = *x
    end_y = *y

    maze[beg_x - 1][beg_y] = PATH
    maze[end_x + 1][end_y] = PATH

    *x = beg_x
    *y = beg_y
}

func delete_openings() {
    maze[beg_x - 1][beg_y] = WALL
    maze[end_x + 1][end_y] = WALL
}

func search_best_openings(x, y *int) {
    best_path_len := 0
    best_turn_cnt := 0
    best_start    := 2
    best_finish   := 2

    for i := 0; i < width; i++ {
        for j := 0; j < width; j++ {
            start  := 2*(i + 1)
            finish := 2*(j + 1)
            *x = start
            *y = finish
            if maze[beg_x][start  - 1] != WALL && maze[beg_x][start  + 1] != WALL {; continue; }
            if maze[end_x][finish - 1] != WALL && maze[end_x][finish + 1] != WALL {; continue; }
            create_openings(x, y)
            solve_maze(x, y)
            if path_len >  best_path_len ||
              (path_len == best_path_len &&
               turn_cnt >  best_turn_cnt) {
               best_start      = start
               best_finish     = finish
               best_turn_cnt   = turn_cnt
               best_path_len   = path_len
               max_path_length = path_len
            }
            restore_maze()
            delete_openings()
            num_solves++
        }
    }
    *x = best_start
    *y = best_finish
    create_openings(x, y)
}

func mid_wall_opening(x, y int) bool {
    return maze[x    ][y    ] == PATH &&
           maze[x - 1][y - 1] != WALL &&
           maze[x - 1][y + 1] != WALL &&
           maze[x + 1][y - 1] != WALL &&
           maze[x + 1][y + 1] != WALL
}

func push_mid_wall_openings() int {
    moves := 0;

    for i := 1; i < 2 * (height + 1); i++ {
        for j := (i & 1) + 1; j < 2 * (width + 1); j += 2 {
            if (mid_wall_opening(i, j)) {
                mark_cell(i, j, WALL)
                if is_odd(i) {; mark_cell(i, j + 2, PATH)	// push right
				} else {;       mark_cell(i + 2, j, PATH)	// push down
				}
				moves++
				num_wall_push++
            }
        }
    }
    if delay != 0 {
        print_maze()
	}
    return moves
}

func create_maze(x, y *int) {
    max_checks = 0
    maze_len   = 0
    num_paths  = 0

    initialize_maze(x, y)
    for {
        num_paths++
        carve_path(x, y)

		if !find_path_start(x, y) {
			break;
		}
    }
    for ; push_mid_wall_openings() != 0 ; {
	}
    delay = 0	// don't print updates while solving for best openings
    search_best_openings(x, y)
}

func main() {
	flag.Usage = func() {
		fmt.Printf("%s\nUsage: %s [options]\n%s", UTS_SIGN_ON, flag.Arg(0),
			 "Options:"                                                                                 + "\n" +
			 "  -f, --fps     <frames per second>  Set refresh rate           (default: none, instant)" + "\n" +
			 "  -h, --height  <height>             Set maze height            (default: screen height)" + "\n" +
			 "  -w, --width   <width>              Set maze width             (default: screen width )" + "\n" +
			 "  -d, --depth   <depth>              Set path search depth      (default: 1            )" + "\n" +
			 "  -p, --path    <length>             Set minimum path length    (default: 0            )" + "\n" +
			 "  -r, --random  <seed>               Set random number seed     (default: current usec )" + "\n" +
			 "  -s, --show                         Show intermediate results while path length not met" + "\n" +
			 "  -b, --blank                        Show empty maze as blank vs. lattice work of walls " + "\n\n")
	}
	rows, cols   := get_console_size()
	max_height   := min(MAX_HEIGHT, (rows - 3)/2)
    max_width    := min(MAX_WIDTH , (cols - 1)/4)
	my_stdout     = bufio.NewWriterSize(os.Stdout, rows*cols)

	flag.IntVar( &fps    , "fps"   , 0         , "refresh rate"            );
	flag.IntVar( &fps    , "f"     , 0         , "refresh rate (shorthand)");
	flag.IntVar( &height , "height", max_height, "maze height"             );
	flag.IntVar( &height , "h"     , max_height, "maze height  (shorthand)");
	flag.IntVar( &width  , "width" , max_width , "maze width"              );
	flag.IntVar( &width  , "w"     , max_width , "maze width   (shorthand)");
	flag.IntVar( &depth  , "depth" , 0         , "search depth"            );
	flag.IntVar( &depth  , "d"     , 0         , "search depth (shorthand)");
	flag.IntVar( &min_len, "path"  , 0         , "path length"             );
	flag.IntVar( &min_len, "p"     , 0         , "path length  (shorthand)");
	flag.IntVar( &seed   , "random", 0         , "random seed"             );
	flag.IntVar( &seed   , "r"     , 0         , "random seed  (shorthand)");
	flag.BoolVar(&show   , "show" , true       , "show working"            );
	flag.BoolVar(&show   , "s"    , true       , "show working (shorthand)");
	flag.BoolVar(&blank  , "blank", true       , "blank walls"             );
	flag.BoolVar(&blank  , "b"    , true       , "blank walls  (shorthand)");

	flag.Parse()

    if depth  <  0 || depth  > 100        {; depth  = 100       ;}
    if fps    <  0 || fps    > 100000     {; fps    = 100000    ;}
    if height <= 0 || height > max_height {; height = max_height;}
    if width  <= 0 || width  > max_width  {; width  = max_width ;}

    if min_len <  0 || min_len >= height * width {
       min_len =  0
	}
    if min_len == 0 {
       min_len = min((height * width) / 2, int(math.Sqrt(float64(height * width))) * 10)
	}
    clr_screen()
    set_cursor_off()

    for {
		switch {
			case fps ==    0: delay = 0
		    case fps <= 1000: delay = 1000 / fps
		    default:          delay = 1000000 / fps
		}

		num_maze_created++
		if (num_maze_created > 1 || seed == 0) {
            seed = time.Now().Nanosecond()
        }
        rand.Seed(int64(seed));

		var path_start_x int
     	var path_start_y int

        create_maze(&path_start_x, &path_start_y); if show {; print_maze(); ms_sleep(1000); }
         solve_maze(&path_start_x, &path_start_y); if show {; print_maze(); ms_sleep(1000); }

		if max_path_length >= min_len {
		   break
		}
    }
    print_maze()
    set_cursor_on()
    putchar('\n')
	my_stdout.Flush()
}


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
 * Rev 2.0 -- translated to go (and converted to camelCase)
 * Rev 2.1 -- added multi-threaded generation
 * Rev 2.2 -- improved (more efficient) look ahead
 */
package main

import (
    "os"
    "fmt"
    "bufio"
    "flag"
    "time"
    "math/rand"
    "sync/atomic"
    "golang.org/x/crypto/ssh/terminal"
)

const (
    version      = "2.2"
    utsSignOn    = "\n" + "Maze Generation Console Utility "+ version +
                   "\n" + "Copyright (c) 2016-2020" +
                   "\n\n"
    blankLine    = "                                                  ";

    maxWidth     = 300
    maxHeight    = 100
    maxXSize     = ((maxHeight + 1)*2 + 1)
    maxYSize     = ((maxWidth  + 1)*2 + 1)

    path         = 0
    wall         = 1
    solved       = 2
    tried        = 3
    check        = 4

    up           = 1
    down         = 2
    left         = 3
    right        = 4

    noSearch     = false
    search       = true

    noUpdate     = false
    update       = true

    blank        = ' '  // ' '
    block        = 0x61 // '#'
    rightBottom  = 0x6a // '+'
    rightTop     = 0x6b // '+'
    leftTop      = 0x6c // '+'
    leftBottom   = 0x6d // '+'
    intersection = 0x6e // '+'
    horizontal   = 0x71 // '-'
    rightTee     = 0x74 // '+'
    leftTee      = 0x75 // '+'
    upTee        = 0x76 // '+'
    downTee      = 0x77 // '+'
    vertical     = 0x78 // '|'
)

type dirTable struct {
    x       int
    y       int
    heading int
}

var (
    stdDirection = [4]dirTable { { 2,  0, down },
                                 {-2,  0, up   },
                                 { 0,  2, right},
                                 { 0, -2, left } }

    outputLookup = [16]byte { blank     , vertical   , horizontal, leftBottom  ,
                              vertical  , vertical   , leftTop   , rightTee    ,
                              horizontal, rightBottom, horizontal, upTee       ,
                              rightTop  , leftTee    , downTee   , intersection }

    simpleLookup = [16]byte { ' ', '|', '-', '+',
                              '|', '|', '+', '+',
                              '-', '+', '-', '+',
                              '+', '+', '+', '+' }

    maze[maxXSize][maxYSize]  int32

    blankFlag         bool
    showFlag          bool
    viewFlag          bool
    checkFlag         bool

    maxX, maxY        int
    begX, endX        int
    begY, endY        int
    width             int
    height            int
    delay             int
    fps               int
    updates           int
    minLen            int
    pathLen           int
    turnCnt           int
    solves            int
    depth             int
    threads           int
    seed              int

    mazeLen           int32
    numPaths          int32
    numSolves         int32
    numThreads        int32
    numWallPush       int32
    numMazeCreated    int32
    numCheckExceeded  int32
    maxChecks         int32
    dspLength         int32
    dspNumChecks      int32
    solveLength       int32
    sumsolveLength    int32

    myStdout          *bufio.Writer
    curFunc           string
    outputName        string
    displayChan       chan struct{}
    finishChan        chan struct{}
)

func msSleep(n   int)       {; time.Sleep(time.Duration(int64(n) * 1000 * 1000)); }

func bool2int(b bool) int   {; if b      {; return 1; }; return 0; }
func min(x, y    int) int   {; if x <  y {; return x; }; return y; }
func max(x, y    int) int   {; if x >  y {; return x; }; return y; }
func nzInt32(x int32) int32 {; if x != 0 {; return x; }; return 1; }

func isEven(x    int) bool  {; return (x & 1) == 0; }
func isOdd( x    int) bool  {; return (x & 1) != 0; }

func putchar(c byte)        {; myStdout.WriteByte(c); }

func setPosition(x, y int)  {; fmt.Fprintf(myStdout, "\033[%d;%dH", x, y); myStdout.Flush(); }
func setLineDraw()          {; fmt.Fprintf(myStdout, "\033(0"           ); myStdout.Flush(); }
func clrLineDraw()          {; fmt.Fprintf(myStdout, "\033(B"           ); myStdout.Flush(); }
func setCursorOff()         {; fmt.Fprintf(myStdout, "\033[?25l"        ); myStdout.Flush(); }
func setCursorOn()          {; fmt.Fprintf(myStdout, "\033[?25h"        ); myStdout.Flush(); }
func clrScreen()            {; fmt.Fprintf(myStdout, "\033[2J"          ); myStdout.Flush(); }
func setSolved()            {; fmt.Fprintf(myStdout, "\033[32m\033[1m"  ); myStdout.Flush(); }
func clrSolved()            {; fmt.Fprintf(myStdout, "\033[30m\033[0m"  ); myStdout.Flush(); }
func setChecked()           {; fmt.Fprintf(myStdout, "\033[31m\033[1m"  ); myStdout.Flush(); }
func clrChecked()           {; fmt.Fprintf(myStdout, "\033[30m\033[0m"  ); myStdout.Flush(); }

// getConsoleSize returns the number of rows and columns available in the current terminal window.
// Defaults to 24 rows and 80 columns if the underlying system call fails.
func getConsoleSize() (int, int) {
    cols, rows, err := terminal.GetSize(0)
    if err != nil {
        rows = 24
        cols = 80
    }
    return rows, cols
}

// initializeMaze sets the entire maze to walls and creates a path around the perimeter to bound the maze.
// The maximum x, y values are set, and the initial x, y values are set to random values.
func initializeMaze(x, y *int) {
    maxX = 2*(height + 1) + 1
    maxY = 2*(width  + 1) + 1

    for i := 1; i < maxX - 1; i++ {
        for j := 1; j < maxY - 1; j++ {
            maze[i][j] = wall
        }
    }
    for i := 0; i < maxX; i++ {; maze[i][0] = path; maze[i][2*(width  + 1)] = path; }
    for j := 0; j < maxY; j++ {; maze[0][j] = path; maze[2*(height + 1)][j] = path; }

    *x = 2*((rand.Intn(height)) + 1)   // random location
    *y = 2*((rand.Intn(width )) + 1)   // for first path

    begX = 2                           // these will
    endX = 2*height                    // never change
}

// restoreMaze returns the maze to a pre-solved state by changing solved or tried cells back to paths.
func restoreMaze()  {
    for i := 0; i < maxX; i++ {
        for j := 0; j < maxY; j++ {
            if (maze[i][j] == solved ||
                maze[i][j] == tried) {
                maze[i][j] =  path
            }
        }
    }
}

// isWall returns true if a cell contains a wall character or a check character (to hide look ahead checks during display)
func isWall(cell int32) bool {
    return cell == wall || (!checkFlag && cell == check)
}

// outputAsciiMaze outputs the maze in ascii format to a text file
func outputAsciiMaze() {
    if outputName != "" {
        f, err := os.Create(outputName)
        if err != nil {
            fmt.Fprintf(myStdout, "Error opening output file: ", err)
            myStdout.Flush()
        } else {
            outFile := bufio.NewWriterSize(f, maxX * maxY)
            fmt.Fprintf(outFile, "%d %d\n", height, width)
            outFile.Flush()
            for i := 1; i < maxX - 1; i++ {
                for j := 1; j < maxY - 1; j++ {
                    switch maze[i][j] {
                        case wall  : if isOdd(i) && isOdd(j) {; fmt.Fprintf(outFile, "%c", simpleLookup[1 * bool2int(maze[i-1][j] == wall && (maze[i-1][j-1] != wall || maze[i-1][j+1] != wall)) +    // wall intersection point
                                                                                                        2 * bool2int(maze[i][j+1] == wall && (maze[i-1][j+1] != wall || maze[i+1][j+1] != wall)) +    // check that there is a path on the diagonal
                                                                                                        4 * bool2int(maze[i+1][j] == wall && (maze[i+1][j-1] != wall || maze[i+1][j+1] != wall)) +
                                                                                                        8 * bool2int(maze[i][j-1] == wall && (maze[i-1][j-1] != wall || maze[i+1][j-1] != wall))])
                                     } else if      isOdd(i) {; fmt.Fprintf(outFile, "-")
                                     } else {                 ; fmt.Fprintf(outFile, "|"); }
                        case path  :                            fmt.Fprintf(outFile, " ")
                        case tried :                            fmt.Fprintf(outFile, ".")
                        case solved:                            fmt.Fprintf(outFile, "*")
                        case check :                            fmt.Fprintf(outFile, "#")
                        default    :                            fmt.Fprintf(outFile, "?")
                    }
                }
                fmt.Fprintf(outFile, "\n")
                outFile.Flush()
            }
            f.Close()
        }
    }
}

// displayMaze displays the current maze within the terminal window using VT100 line drawing characters.
func displayMaze()  {
    setPosition(0, 0)
    setLineDraw()

    for i := 1; i < maxX - 1; i++ {
        for j := 1; j < maxY - 1; j++ {
            var vertexChar, solvedChar, leftChar, rightChar, wallChar byte

            if isOdd(i) && isOdd(j) {
                vertexChar = outputLookup[1 * bool2int(isWall(maze[i-1][j]) && (!isWall(maze[i-1][j-1]) || !isWall(maze[i-1][j+1]))) +    // wall intersection point
                                          2 * bool2int(isWall(maze[i][j+1]) && (!isWall(maze[i-1][j+1]) || !isWall(maze[i+1][j+1]))) +    // check that there is a path on the diagonal
                                          4 * bool2int(isWall(maze[i+1][j]) && (!isWall(maze[i+1][j-1]) || !isWall(maze[i+1][j+1]))) +
                                          8 * bool2int(isWall(maze[i][j-1]) && (!isWall(maze[i-1][j-1]) || !isWall(maze[i+1][j-1])))];
            } else {
                vertexChar = outputLookup[1 * bool2int(isWall(maze[i-1][j]) && (!isWall(maze[i  ][j-1]) || !isWall(maze[i  ][j+1]))) +    // non-intersection point
                                          2 * bool2int(isWall(maze[i][j+1]) && (!isWall(maze[i-1][j  ]) || !isWall(maze[i+1][j  ]))) +    // check that there is a path adjacent
                                          4 * bool2int(isWall(maze[i+1][j]) && (!isWall(maze[i  ][j-1]) || !isWall(maze[i  ][j+1]))) +
                                          8 * bool2int(isWall(maze[i][j-1]) && (!isWall(maze[i-1][j  ]) || !isWall(maze[i+1][j  ])))];
            }
                solvedChar = outputLookup[1 * bool2int(maze[i-1][j] == maze[i][j]) +
                                          2 * bool2int(maze[i][j+1] == maze[i][j]) +
                                          4 * bool2int(maze[i+1][j] == maze[i][j]) +
                                          8 * bool2int(maze[i][j-1] == maze[i][j])]

            if isEven(i) && (maze[i][j-1] == solved || maze[i][j-1] == check) {;  leftChar = horizontal; } else {;  leftChar = blank; }
            if isEven(i) && (maze[i][j+1] == solved || maze[i][j+1] == check) {; rightChar = horizontal; } else {; rightChar = blank; }

            if blankFlag {; wallChar = vertexChar; } else {; wallChar = solvedChar; }

            switch {
                case maze[i][j] == solved  :                 setSolved();  putchar(leftChar); if (isEven(j)) {; putchar(solvedChar); putchar(rightChar); }; clrSolved()
                case maze[i][j] == check   : if checkFlag {; setChecked(); putchar(leftChar); if (isEven(j)) {; putchar(solvedChar); putchar(rightChar); }; clrChecked();
                                             } else       {;               putchar(blank   ); if (isEven(j)) {; putchar(blank     ); putchar(blank    ); }}
                case isEven(i) && isEven(j):                               putchar(blank   ); if (isEven(j)) {; putchar(blank     ); putchar(blank    ); }
                case maze[i][j] == wall    :                               putchar(wallChar); if (isEven(j)) {; putchar(  wallChar); putchar( wallChar); }
                default                    :                               putchar(blank   ); if (isEven(j)) {; putchar(blank     ); putchar(blank    ); }
            }
        }
        putchar('\n')
    }
    clrLineDraw()
    updates++;

    fmt.Fprintf(myStdout, "updates=%d, height=%d, width=%d, seed=%d, num_wall_push=%d, num_maze_created=%d, num_solves=%d, avg_solve_length=%d, solve_length=%d, avg_path_length=%d, num_paths=%d, maze_len=%d, threads=%d, length=%d, checks=%d, max_checks=%d, checks_exceeded=%d %s\r",
                           updates   , height   , width   , seed   ,
                           atomic.LoadInt32(&numWallPush     ),
                           atomic.LoadInt32(&numMazeCreated  ),
                           atomic.LoadInt32(&numSolves       ),
                           atomic.LoadInt32(&sumsolveLength  ) /
                   nzInt32(atomic.LoadInt32(&numMazeCreated  )),
                           atomic.LoadInt32(&solveLength     ),
                           atomic.LoadInt32(&mazeLen         ) /
                   nzInt32(atomic.LoadInt32(&numPaths        )),
                           atomic.LoadInt32(&numPaths        ),
                           atomic.LoadInt32(&mazeLen         ),
                           atomic.LoadInt32(&numThreads      ),
                           atomic.LoadInt32(&dspLength       ),
                           atomic.LoadInt32(&dspNumChecks    ),
                           atomic.LoadInt32(&maxChecks       ),
                           atomic.LoadInt32(&numCheckExceeded),
                           blankLine);
    outputAsciiMaze()
}

// displayRoutine waits to receive a signal on displayChan and then prints the maze
func displayRoutine () {
    for range displayChan {
        displayMaze()
    }
}

// updateMaze signals displayChan if there is no pending signal and then sleeps delay ms if non-zero
func updateMaze(length, numChecks int) {
    if numChecks != 0 {
        atomic.StoreInt32(&dspNumChecks, int32(numChecks))
    }
    select {
        case displayChan <- struct{}{}:
        default:
    }
    if delay > 0 {
        msSleep(delay)
    }
}

// markCell sets a location x, y inside the maze array to the value (wall, path, solved, tried)
// It also displays the maze if delay is non-zero and the frame rate is less than 1000/sec
// and only then for cells at locations with even x, y coordinates (to reduce number of refreshes)
func markCell(x, y int, value int32, update bool, length, numChecks int) bool {
    if maze[x][y] == check || maze[x][y] == value {
        return false
    }
    priorValue := atomic.SwapInt32(&maze[x][y], value)
    if priorValue == check {
        maze[x][y] = check
        return false
    }
    if priorValue == value {
        return false
    }
    if (update || (checkFlag && maze[x][y] == check)) && delay > 0 && fps <= 1000 && isEven(x) && isEven(y) {
        updateMaze(length, numChecks)
    }
    return true
}

// checkDirections recursively checks to see if a path of a given length can be carved or traced from the given x, y location.
// (limited to a total of 1/2 million checks)
func checkDirections(x, y, dx, dy, limit int, value int32, length, minLength, checks, numChecks *int) bool {
    if *length < 0 {
        return true
    }
    if *checks >= limit {
        atomic.AddInt32(&numCheckExceeded, 1)
        return false
    }
    if x + dx <= 1 || y + dy <= 1 || maze[x + dx][y + dy] != value || !markCell(x + dx/2, y + dy/2, check, checkFlag, *length, *numChecks) {
        return false
    }
    if !markCell(x + dx, y + dy, check, checkFlag, *length, *numChecks) {
        maze[x + dx/2][y + dy/2] = value
        return false
    }
    *length--
    *checks++
    *numChecks++

    offset := rand.Intn(4)
    match  := false
    for  i := 0; i < 4; i++ {
        dir := &stdDirection[(i + offset) % 4]
        dirLength := *length
        if maze[x + dx + dir.x/2][y + dy + dir.y/2] == value && maze[x + dx + dir.x][y + dy + dir.y] == value && checkDirections(x + dx, y + dy, dir.x, dir.y, limit, value, &dirLength, minLength, checks, numChecks) {
           *length = dirLength
           match = true
           break
        }
    }
    if *minLength > *length {
       *minLength = *length
    }
    *length++

    maze[x + dx  ][y + dy  ] =  value
    maze[x + dx/2][y + dy/2] =  value
    return match
}

// orphan1x1 returns true if a location is surrounded by walls on all 4 sides and paths on the other side of all those walls.
func orphan1x1(x, y int) bool {
    return      x > 1             &&      y > 1             &&  // bounds check
           maze[x + 1][y] == wall && maze[x + 2][y] == path &&  // vertical (look down & up)
           maze[x - 1][y] == wall && maze[x - 2][y] == path &&
           maze[x][y + 1] == wall && maze[x][y + 2] == path &&  // horizontal (right & left)
           maze[x][y - 1] == wall && maze[x][y - 2] == path
}

// checkOrphan returns true if carving a path at a given location x,y in a given direction dx, dy
// would create a 1x1 orphan left, right, above, or below the path.
func checkOrphan(x, y, dx, dy, length int) bool {
    orphan := false;
    if      x > 1  && y > 1   && length > 0 && length == depth  &&  // this only makes sense when carving paths, not when solving, and only if we haven't exhausted our search depth
       maze[x + dx  ][y + dy  ] ==  wall                        &&
       maze[x + dx/2][y + dy/2] ==  wall                        &&
       markCell(x + dx  , y + dy  , path, noUpdate, length, 0)  &&  // temporarily set new path
       markCell(x + dx/2, y + dy/2, path, noUpdate, length, 0) {

        orphan = orphan1x1(x + dx + 2, y + dy    ) ||   // check for 1x1 orphans below & above of the new location
                 orphan1x1(x + dx - 2, y + dy    ) ||
                 orphan1x1(x + dx    , y + dy + 2) ||   // check for 1x1 orphans right & left  of the new location
                 orphan1x1(x + dx    , y + dy - 2)

        maze[x + dx  ][y + dy  ] = wall                 // restore original walls
        maze[x + dx/2][y + dy/2] = wall
    }
    return orphan
}

// look returns 1 if at a given location x, y a path of a given length can be carved or traced in a given direction dx, dy without creating 1x1 orphans.
// The direction (heading, dx, dy) is stored in the direction table directions if the path can be created.
func look(heading, x, y, dx, dy, num int, value int32, directions []dirTable, length, minLength, numChecks *int) int {
    checks := 0
    if      x > 1  && y > 1              &&
       maze[x + dx/2][y + dy/2] == value &&
       maze[x + dx  ][y + dy  ] == value && !checkOrphan(x, y, dx, dy, *length) && checkDirections(x, y, dx, dy, 10*(depth + 1), value, length, minLength, &checks, numChecks) {
        directions[num].x = dx
        directions[num].y = dy
        directions[num].heading = heading
        return 1
    }
    return 0
}

// findDirections returns the number of directions that a path can be carved or traces from a given location x, y.
// The path length requirement of length is enforced.
func findDirections(x, y int, length *int, value int32, directions []dirTable) int {
    num       := 0
    numChecks := 0
    if value != wall || (maze[x][y] == path && markCell(x, y, check, noUpdate, *length, numChecks)) {
        minLength := [4]int {*length, *length, *length, *length}
        len := *length
        for {
            atomic.StoreInt32(&dspLength, int32(len))
            dirLength := [4]int {len, len, len, len}
            offset    := rand.Intn(4)
            for i := 0; i < 4; i++ {
                dir := &stdDirection[(i + offset) % 4]
                num += look(dir.heading, x, y, dir.x, dir.y, num, value, directions, &dirLength[i] , &minLength[i], &numChecks)
            }
            if num > 0 || len < 0 {
               break
            }
            minLength := min( min(minLength[0], minLength[1]), min(minLength[2], minLength[3]) )
            if minLength <= 0 {
               minLength = 1
            }
            len -= minLength
        }
        if len == *length && len < depth {
           len++
        }
        *length = len
        if maze[x][y] == check {
           maze[x][y] =  path
        }
    }
    if atomic.LoadInt32( &maxChecks) < int32(numChecks)  {
       atomic.StoreInt32(&maxChecks  , int32(numChecks))
    }
    return (num);
}

// straightThru returns true if the path at the given location x, y has a path left and right of it, or above and below it
func straightThru(x, y int, value int32) bool {
    return        x > 1              &&      y > 1               &&
           ((maze[x - 1][y] == value && maze[x - 2][y] == value  &&  // vertical   (look up & down)
             maze[x + 1][y] == value && maze[x + 2][y] == value) ||
            (maze[x][y - 1] == value && maze[x][y - 2] == value  &&  // horizontal (look left & right)
             maze[x][y + 1] == value && maze[x][y + 2] == value))
}

// findPathStart starts looking at a random x, y location for a position along an existing non-straight through path that can start a new path of at least depth length
func findPathStart(x, y *int) bool {
    directions := make([]dirTable, 4, 4)
    length := 0
    xStart := rand.Intn(height)
    yStart := rand.Intn(width )
    for  i := 0; i < height; i++ {
        for j := 0; j < width; j++ {
            len := length
            *x = 2*((xStart + i) % height + 1)
            *y = 2*((yStart + j) % width  + 1)
            if (maze[*x][*y] == path && !straightThru(*x, *y, path) && findDirections(*x, *y, &len, wall, directions) > 0) {
                return true
            }
        }
    }
    return false
}

// carvePath carves a new path in the maze starting at location x, y
// It does this by repeatedly determining the number of possible directions to move
// and then randomly choosing one of them and then marking the new cells on the path
func carvePath(x, y *int) bool {
    directions := make([]dirTable, 4, 4)
    pathLen    := 0
    length     := depth
    atomic.AddInt32(&numPaths, 1)
    markCell(*x, *y, path, noUpdate, 0, 0)
    for {
        num    := findDirections(*x, *y, &length, wall, directions)
        if num == 0 {
           break
        }
        dir := rand.Intn(num)
        if !markCell(*x +  directions[dir].x/2, *y +  directions[dir].y/2, path, update, 0, 0) {
            continue
        }
        if !markCell(*x +  directions[dir].x  , *y +  directions[dir].y  , path, update, 0, 0) {
            markCell(*x +  directions[dir].x/2, *y +  directions[dir].y/2, wall, update, 0, 0)
               continue
        }
        *x += directions[dir].x
        *y += directions[dir].y
        atomic.AddInt32(&mazeLen, 1)
        pathLen++
    }
    if delay > 0 {
        updateMaze(0, 0)
    }
    if atomic.LoadInt32(&numThreads) < int32(threads) {
       atomic. AddInt32(&numThreads, 1)
       go carveRoutine()
    }
    return pathLen > 0
}

// followPath follows a path in the maze starting at location x, y
// It does this by repeatedly determining if there are any possible directions to move
// and then choosing the first of them and then marking the new cells on the path as solved
func followPath(x, y *int) bool {
    directions := make([]dirTable, 4, 4)
    lastDir    :=  0
    length     := -1
    markCell(*x, *y, solved, noUpdate, 0, 0)
    for begX  <= *x && *x <= endX && findDirections(*x, *y, &length, path, directions) > 0 {
        markCell(*x +  directions[0].x  , *y +  directions[0].y  , solved, update, 0, 0)
        markCell(*x +  directions[0].x/2, *y +  directions[0].y/2, solved, update, 0, 0)
                 *x += directions[0].x  ; *y += directions[0].y
        pathLen++
        if (lastDir != directions[0].heading)  {
            lastDir  = directions[0].heading
            turnCnt++
        }
    }
    return *x > endX
}

// backTrackPath backtracks a path in the maze starting at location x, y
// It does this by repeatedly determining if there are any possible directions to move
// and then choosing the first of them and then marking the new cells on the path as tried (not solved)
func backTrackPath(x, y *int) {
    directions := make([]dirTable, 4, 4)
    lastDir    :=  0
    length     := -1
    markCell(*x, *y, tried, noUpdate, 0, 0)
    for findDirections(*x, *y, &length, path, directions) == 0 && findDirections(*x, *y, &length, solved, directions) > 0 {
        markCell(*x +  directions[0].x  , *y +  directions[0].y  , tried, update, 0, 0)
        markCell(*x +  directions[0].x/2, *y +  directions[0].y/2, tried, update, 0, 0)
                 *x += directions[0].x  ; *y += directions[0].y
        pathLen--
        if (lastDir != directions[0].heading)  {
            lastDir  = directions[0].heading;
            turnCnt--
        }
    }
}

// solveMaze solves a maze by starting at the beginning and following each path,
// back tracking when they dead end, until the end of the maze is found.
func solveMaze(x, y *int) {
    saveCheck := checkFlag
    checkFlag  = false
    saveDepth := depth
    depth      = -1
    pathLen    = 0
    turnCnt    = 0

    maze[begX - 1][begY] = solved
    for  !followPath(x, y) {
       backTrackPath(x, y)
    }
    maze[endX + 1][endY] = solved
    checkFlag = saveCheck
    depth     = saveDepth
    solves++
}

// createOpenings marks the top and bottom of the maze at locations begX, x and endX, y as paths
// and then sets x, y to the start of the maze: begX, begY.
func createOpenings(x, y *int) {
    begY = *x
    endY = *y

    maze[begX - 1][begY] = path
    maze[endX + 1][endY] = path

    *x = begX
    *y = begY
}

// deleteOpenings marks the openings in the maze by setting the locations begX, begY and endX, endY back to wall.
func deleteOpenings()  {
    maze[begX - 1][begY] = wall
    maze[endX + 1][endY] = wall
}

// searchBestOpenings sets the top an bottom openings to all possible locations and repeatedly solves the maze
// keeping track of which set of openings produces the longest solution path, then sets x, y to the result.
func searchBestOpenings(x, y *int) {
    bestPathLen := 0
    bestTurnCnt := 0
    bestStart   := 2
    bestFinish  := 2
    saveDelay   := delay    // don't print updates while solving for best openings
    delay        = 0

    for i := 0; i < width; i++ {
        for j := 0; j < width; j++ {
            start  := 2*(i + 1)
            finish := 2*(j + 1)
            *x = start
            *y = finish
            if maze[begX][start  - 1] != wall && maze[begX][start  + 1] != wall {; continue; }
            if maze[endX][finish - 1] != wall && maze[endX][finish + 1] != wall {; continue; }
            createOpenings(x, y)
            solveMaze(x, y)
            if pathLen  >  bestPathLen ||
              (pathLen  == bestPathLen &&
               turnCnt  >  bestTurnCnt) {
               bestStart     = start
               bestFinish    = finish
               bestTurnCnt   = turnCnt
               bestPathLen   = pathLen
               atomic.StoreInt32(&solveLength, int32(pathLen))
            }
            restoreMaze()
            deleteOpenings()
            atomic.AddInt32(&numSolves, 1)
        }
    }
    atomic.AddInt32(&sumsolveLength, atomic.LoadInt32(&solveLength))
    if viewFlag {       // only restore delay value if view solve flag is set
        delay = saveDelay
    }
    *x    = bestStart
    *y    = bestFinish
    createOpenings(x, y)
}

// midWallOpening returns true if there is a mid wall (non-corner) opening in a path at location x, y
func midWallOpening(x, y int) bool {
    return     x > 0 && y > 0         &&
           maze[x    ][y    ] == path &&
           maze[x - 1][y - 1] != wall &&
           maze[x - 1][y + 1] != wall &&
           maze[x + 1][y - 1] != wall &&
           maze[x + 1][y + 1] != wall
}

// pushMidWallOpenings loops over all locations in the maze searching for mid wall openings and pushes horizontal
// openings to the right, and vertical openings down, and then returns the number of mid wall openings moved.
func pushMidWallOpenings() {
    for {
        moves := 0
        for i := 1; i < 2 * (height + 1); i++ {
            for j := (i & 1) + 1; j < 2 * (width + 1); j += 2 {
                if (midWallOpening(i, j)) {
                    markCell(i, j, wall, noUpdate, 0, 0)
                    if isOdd(i) {; markCell(i,  j + 2, path, update, 0, 0)   // push right
                    } else {;      markCell(i + 2,  j, path, update, 0, 0)   // push down
                    }
                    moves++
                    atomic.AddInt32(&numWallPush, 1)
                }
            }
        }
        if delay > 0 {
            updateMaze(0, 0)
        }
        if moves == 0 {
            break
        }
    }
}

// carvePaths continuuosly carves new paths while it can find starting locations for new paths
func carvePaths(x, y int) {

    if x > 0 && y > 0 {
        carvePath(&x, &y)
    }
    for findPathStart(&x, &y) &&
            carvePath(&x, &y) {
    }
}

// carveRoutine calls carvePaths and sends a signal finshChan when complete
func carveRoutine() {
    msSleep(10)
    carvePaths(0, 0)
    finishChan <- struct{}{}
}

// createMaze initializes the maze array then repeatedly carves new paths until no new path starting locations can be found.
// Following this it then repeatedly pushes mid wall openings right or down until there are no longer any mid wall openings.
// Lastly it searches for the best openings, top and bottom, to create the maze with the longest solution path.
func createMaze(x, y *int) {
    atomic.StoreInt32(&maxChecks       , 0)
    atomic.StoreInt32(&mazeLen         , 0)
    atomic.StoreInt32(&numPaths        , 0)
    atomic.StoreInt32(&numCheckExceeded, 0)

    initializeMaze(x, y)
    carvePaths(*x, *y)
    for i := 0; int32(i) < atomic.LoadInt32(&numThreads); i++ {
        <- finishChan
    }
    pushMidWallOpenings()
    searchBestOpenings(x, y)
}

// maze main parses the command line switches and then repeatedly creates and
// solves mazes until the minimum solution path length criteria is met.
func main() {
    flag.Usage = func() {
        fmt.Printf("%s\nUsage: %s [options]\n%s", utsSignOn, flag.Arg(0),
             "Options:"                                                                                 + "\n" +
             "  -f, --fps     <frames per second>  Set refresh rate           (default: none, instant)" + "\n" +
             "  -h, --height  <height>             Set maze height            (default: screen height)" + "\n" +
             "  -w, --width   <width>              Set maze width             (default: screen width )" + "\n" +
             "  -t, --threads <threads>            Set maze path thread count (default: 0            )" + "\n" +
             "  -d, --depth   <depth>              Set path search depth      (default: 0            )" + "\n" +
             "  -p, --path    <length>             Set minimum path length    (default: 0            )" + "\n" +
             "  -r, --random  <seed>               Set random number seed     (default: current usec )" + "\n" +
             "  -s, --show                         Show intermediate results while path length not met" + "\n" +
             "  -v, --view                         Show intermediate results determining maze solution" + "\n" +
             "  -l, --look                         Show look ahead path searches while creating maze  " + "\n" +
             "  -b, --blank                        Show empty maze as blank vs. lattice work of walls " + "\n" +
             "  -o, --output  <filename>           Output portable ASCII encoded maze when completed  " + "\n\n")
    }
    rows, cols := getConsoleSize()
    maxHeight  := min(maxHeight, (rows - 3)/2)
    maxWidth   := min(maxWidth , (cols - 1)/4)
    myStdout    = bufio.NewWriterSize(os.Stdout, rows*cols)
    displayChan = make(chan struct{});
    finishChan  = make(chan struct{});

    flag.IntVar(   &fps       , "fps"    , 0        , "refresh rate"               );
    flag.IntVar(   &fps       , "f"      , 0        , "refresh rate    (shorthand)");
    flag.IntVar(   &height    , "height" , maxHeight, "maze height"                );
    flag.IntVar(   &height    , "h"      , maxHeight, "maze height     (shorthand)");
    flag.IntVar(   &width     , "width"  , maxWidth , "maze width"                 );
    flag.IntVar(   &width     , "w"      , maxWidth , "maze width      (shorthand)");
    flag.IntVar(   &threads   , "threads", 0        , "path threads"               );
    flag.IntVar(   &threads   , "t"      , 0        , "path threads    (shorthand)");
    flag.IntVar(   &depth     , "depth"  , 0        , "search depth"               );
    flag.IntVar(   &depth     , "d"      , 0        , "search depth    (shorthand)");
    flag.IntVar(   &minLen    , "path"   , 0        , "path length"                );
    flag.IntVar(   &minLen    , "p"      , 0        , "path length     (shorthand)");
    flag.IntVar(   &seed      , "random" , 0        , "random seed"                );
    flag.IntVar(   &seed      , "r"      , 0        , "random seed     (shorthand)");
    flag.BoolVar(  &showFlag  , "show"   , false    , "show working"               );
    flag.BoolVar(  &showFlag  , "s"      , false    , "show working    (shorthand)");
    flag.BoolVar(  &viewFlag  , "view"   , false    , "show solving"               );
    flag.BoolVar(  &viewFlag  , "v"      , false    , "show solving    (shorthand)");
    flag.BoolVar(  &checkFlag , "look"   , false    , "show look ahead"            );
    flag.BoolVar(  &checkFlag , "l"      , false    , "show look ahead (shorthand)");
    flag.BoolVar(  &blankFlag , "blank"  , false    , "blank walls"                );
    flag.BoolVar(  &blankFlag , "b"      , false    , "blank walls     (shorthand)");
    flag.StringVar(&outputName, "output" , ""       , "output ascii"               );
    flag.StringVar(&outputName, "o"      , ""       , "output ascii    (shorthand)");

    flag.Parse()

    if depth  <  0 || depth  > 100            {; depth  = 100           ;}
    if fps    <  0 || fps    > 100000         {; fps    = 100000        ;}
    if height <= 0 || height > maxHeight      {; height = maxHeight     ;}
    if width  <= 0 || width  > maxWidth       {; width  = maxWidth      ;}
    if minLen <  0 || minLen > height*width/3 {; minLen = height*width/3;}

    clrScreen()
    setCursorOff()
    go displayRoutine()

    for {
        atomic.StoreInt32(&numThreads, 0)

        switch {
            case fps ==    0: delay = 0
            case fps <= 1000: delay = 1000 / fps
            default:          delay = 1000000 / fps
        }

        atomic.AddInt32(&numMazeCreated, 1)
        if (atomic.LoadInt32(&numMazeCreated) > 1 || seed == 0) {
            seed = time.Now().Nanosecond()
        }
        rand.Seed(int64(seed));

        var pathStartX int
        var pathStartY int

        createMaze(&pathStartX, &pathStartY); if showFlag {; updateMaze(0, 0);  msSleep(1000); }
         solveMaze(&pathStartX, &pathStartY); if showFlag {; updateMaze(0, 0);  msSleep(1000); }

        if atomic.LoadInt32(&solveLength) >= int32(minLen) {
           break
        }
    }
    updateMaze(0, 0)
    msSleep(100)
    restoreMaze()
    outputAsciiMaze()
    setCursorOn()
    putchar('\n')
    myStdout.Flush()
}


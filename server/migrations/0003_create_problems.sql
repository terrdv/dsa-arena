CREATE TABLE problems (
    id INTEGER PRIMARY KEY,
    title TEXT NOT NULL,
    problem_description TEXT NOT NULL,
    testcases JSONB NOT NULL
);

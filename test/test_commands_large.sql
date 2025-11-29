-- Larger test SQL file for compression demonstration
\u roll_back

-- Create a test table
CREATE TABLE IF NOT EXISTS compression_test (
    id INT AUTO_INCREMENT PRIMARY KEY,
    name VARCHAR(255),
    description TEXT,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

-- Insert some test data
INSERT INTO compression_test (name, description) VALUES
('Test Record 1', 'This is a test record with some description text that will help demonstrate compression when the file gets larger'),
('Test Record 2', 'Another test record with a longer description to make the file bigger for better compression demonstration'),
('Test Record 3', 'Yet another record with description text that repeats some information to increase file size'),
('Test Record 4', 'Fourth test record with more descriptive text to ensure we have enough content for meaningful compression'),
('Test Record 5', 'Final test record completing our dataset for the compression demonstration example');

-- Query the data
SELECT * FROM compression_test;

-- Show compression info
SELECT 'This file demonstrates zstd compression support in go-mycli' as info;

-- Clean up
DROP TABLE compression_test;

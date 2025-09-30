const express = require('express');
const axios = require('axios');

const app = express();
const PORT = 3000;

// GET endpoint that fetches from another endpoint
app.get('/fetch', async (req, res) => {
    try {
        // Fetch from a sample endpoint (e.g., httpbin.org)
        const response = await axios.post('http://localhost:8081/query/shells',{
        "$condition": {
            "$eq": [
            { "$field": "$sm#displayName[].language" },
            { "$field": "$sm#description[].language" }
            ]
        }
        });
        res.json({
            message: 'Fetched data successfully',
            data: response.data
        });
    } catch (error) {
        res.status(500).json({
            message: 'Error fetching data',
            error: error.message
        });
    }
});

app.listen(PORT, () => {
    console.log(`Server is running on http://localhost:${PORT}`);
});

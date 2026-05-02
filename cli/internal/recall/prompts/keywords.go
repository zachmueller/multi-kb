package prompts

const KeywordDerivationPrompt = `Extract 3 to 5 specific search keywords from the user's message. These keywords will be used for text search (git grep) against a knowledge base of technical notes.

Choose keywords that are:
- Specific technical terms, library names, API names, or concepts
- Likely to appear in relevant knowledge base notes
- Not generic words like "how", "what", "help", "problem", "issue"

Return ONLY a JSON array of strings, nothing else.

Example input: "How do I configure DynamoDB Global Tables for cross-region replication?"
Example output: ["DynamoDB", "Global Tables", "cross-region", "replication"]

Example input: "What's the best way to handle Go context cancellation in HTTP middleware?"
Example output: ["Go", "context cancellation", "HTTP middleware"]`

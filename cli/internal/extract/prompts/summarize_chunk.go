package prompts

const ChunkSummarizationPrompt = `Summarize the following conversation chunk. This summary will be prepended to the next chunk of the same conversation to provide context for knowledge extraction.

Preserve in your summary:
- Key topics and technical subjects discussed
- Decisions made and their rationale
- Important technical details (versions, configurations, commands)
- The state of any ongoing problem-solving (what was tried, what worked, what's pending)
- Names of systems, services, or components being discussed

Keep the summary between 10,000 and 20,000 tokens. Focus on information that would help understand subsequent messages in the conversation. Omit pleasantries, repetitive debugging attempts, and verbose tool output details.

Write the summary as a structured document with headings and bullet points, not as prose.`

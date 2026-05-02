export interface SubmitKnowledgeBody {
  title: string;
  content: string;
  author: string;
}

type ValidationResult =
  | { valid: true; data: SubmitKnowledgeBody }
  | { valid: false; errors: Record<string, string> };

export function validateSubmitKnowledge(body: unknown): ValidationResult {
  const errors: Record<string, string> = {};
  const b = body as Record<string, unknown>;

  if (typeof b.title !== "string" || b.title.trim().length === 0) {
    errors.title = "title is required and must be a non-empty string";
  } else if (b.title.length > 255) {
    errors.title = "title must be 255 characters or fewer";
  }

  if (typeof b.content !== "string" || b.content.trim().length === 0) {
    errors.content = "content is required and must be a non-empty string";
  } else if (b.content.length > 100_000) {
    errors.content = "content must be 100,000 characters or fewer";
  }

  if (typeof b.author !== "string" || b.author.trim().length === 0) {
    errors.author = "author is required and must be a non-empty string";
  } else if (b.author.length > 100) {
    errors.author = "author must be 100 characters or fewer";
  }

  if (Object.keys(errors).length > 0) {
    return { valid: false, errors };
  }

  return {
    valid: true,
    data: {
      title: (b.title as string).trim(),
      content: b.content as string,
      author: (b.author as string).trim(),
    },
  };
}

import { Avatar, AvatarFallback, AvatarImage } from "@/components/ui/avatar";
import { cn } from "@/lib/utils";

// Thai prepositive vowels (เ แ โ ใ ไ) are written before the consonant they belong to.
const thaiLeadingVowels = /^[เ-ไ]+/;
// A consonant plus the marks that sit above or below it.
const firstLetter = /^[^\sัิ-ฺ็-๎][ัิ-ฺ็-๎]*/u;

/** The letter to show in an avatar: upper-case for Latin, the consonant (with its marks) for Thai. */
export function initialOf(name: string): string {
  const trimmed = name.trim().replace(thaiLeadingVowels, "");
  return trimmed.match(firstLetter)?.[0].toLocaleUpperCase() ?? "?";
}

/** A round avatar: the person's picture when there is one, else their initial. Put the name next to it; the picture or initial is decoration. */
export function UserAvatar({ name, src = "", className }: { name: string; src?: string; className?: string }) {
  return (
    <Avatar className={cn("size-8", className)}>
      {src && <AvatarImage src={src} alt="" />}
      <AvatarFallback className="bg-primary/10 text-xs font-medium text-primary">
        <span aria-hidden="true">{initialOf(name)}</span>
      </AvatarFallback>
    </Avatar>
  );
}
